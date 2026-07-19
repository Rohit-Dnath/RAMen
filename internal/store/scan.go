package store

import (
	"sort"
	"time"
)

// Cursor iteration for SCAN and its typed variants.
//
// Map order is not stable, so the cursor encodes a position in a sorted order
// instead of a count of keys seen. For the keyspace SCAN it packs the shard
// index and the offset into that shard's sorted keys as
// cursor = offset*shardCount + shardIndex; cursor 0 both starts and ends the
// scan. Keys added or removed mid-scan shift offsets, so a key can be missed or
// returned twice (documented in docs/commands.md).

// sortedLiveKeys returns the shard's live keys sorted, taking the shard read
// lock itself so a scan never holds a lock across shards.
func (sh *shard) sortedLiveKeys(now time.Time) []string {
	sh.mu.RLock()
	keys := make([]string, 0, len(sh.m))
	for k, e := range sh.m {
		if !e.expired(now) {
			keys = append(keys, k)
		}
	}
	sh.mu.RUnlock()
	sort.Strings(keys)
	return keys
}

// Scan returns one page of the keyspace: starting from cursor, it examines up to
// count keys across shards and returns those matching the glob pattern (empty
// matches everything) plus the next cursor, which is 0 when done. MATCH is
// applied after the count keys are fetched, so a page can be empty with a
// non-zero cursor; callers loop until the cursor is 0.
func (s *Store) Scan(cursor uint64, match string, count int) (uint64, []string) {
	if count <= 0 {
		count = defaultScanCount
	}
	now := s.now()
	shardIdx := int(cursor % shardCount)
	offset := int(cursor / shardCount)

	var out []string
	examined := 0
	for shardIdx < shardCount {
		keys := s.shards[shardIdx].sortedLiveKeys(now)
		i := offset
		for i < len(keys) && examined < count {
			if match == "" || matchPattern(match, keys[i]) {
				out = append(out, keys[i])
			}
			i++
			examined++
		}
		if i < len(keys) {
			// Stopped mid-shard because the count budget ran out; resume here.
			return uint64(i)*shardCount + uint64(shardIdx), out
		}
		// This shard is exhausted; roll to the next one at offset 0.
		shardIdx++
		offset = 0
		if examined >= count {
			if shardIdx >= shardCount {
				return 0, out
			}
			return uint64(shardIdx), out // offset 0 in the next shard
		}
	}
	return 0, out
}

// defaultScanCount is the COUNT hint used when the client supplies none.
const defaultScanCount = 10

// HScan returns one page of the hash at key as a flat [field, value, ...] slice
// plus the next cursor, keeping only fields matching the pattern. A missing key
// yields cursor 0 and no elements; a wrong-type key returns ErrWrongType.
func (s *Store) HScan(key string, cursor uint64, match string, count int) (uint64, []string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		sh.mu.RUnlock()
		return 0, nil, nil
	}
	h, err := asHash(e)
	if err != nil {
		sh.mu.RUnlock()
		return 0, nil, err
	}
	// Copy the hash under the lock, sort after releasing it: sorting a large
	// hash while holding the shard lock would block every writer to the shard
	// for the whole sort, the same reason sortedLiveKeys releases before sorting.
	cp := make(map[string]string, len(h))
	for f, v := range h {
		cp[f] = v
	}
	sh.mu.RUnlock()

	fields := make([]string, 0, len(cp))
	for f := range cp {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	next, window := scanWindow(fields, cursor, count)
	out := make([]string, 0, len(window)*2)
	for _, f := range window {
		if match == "" || matchPattern(match, f) {
			out = append(out, f, cp[f])
		}
	}
	return next, out, nil
}

// SScan returns one page of the set at key, keeping only members matching the
// pattern, plus the next cursor. A missing key yields cursor 0 and no elements;
// a wrong-type key returns ErrWrongType.
func (s *Store) SScan(key string, cursor uint64, match string, count int) (uint64, []string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		sh.mu.RUnlock()
		return 0, nil, nil
	}
	set, err := asSet(e)
	if err != nil {
		sh.mu.RUnlock()
		return 0, nil, err
	}
	members := make([]string, 0, len(set))
	for m := range set {
		members = append(members, m)
	}
	sh.mu.RUnlock()
	// Sorted outside the lock so a large set does not stall the shard's writers.
	sort.Strings(members)
	next, window := scanWindow(members, cursor, count)
	out := make([]string, 0, len(window))
	for _, m := range window {
		if match == "" || matchPattern(match, m) {
			out = append(out, m)
		}
	}
	return next, out, nil
}

// ZScan returns one page of the sorted set at key, keeping only members matching
// the pattern, plus the next cursor. Members are ordered by name (enough for a
// stable cursor) and the caller formats the scores. A missing key yields cursor
// 0 and no elements; a wrong-type key returns ErrWrongType.
func (s *Store) ZScan(key string, cursor uint64, match string, count int) (uint64, []ZMember, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		sh.mu.RUnlock()
		return 0, nil, nil
	}
	z, err := asZSet(e)
	if err != nil {
		sh.mu.RUnlock()
		return 0, nil, err
	}
	// Copy the scores under the lock, sort after releasing it, so a large zset
	// does not stall the shard's writers for the duration of the sort.
	scores := make(map[string]float64, len(z.scores))
	for m, sc := range z.scores {
		scores[m] = sc
	}
	sh.mu.RUnlock()

	members := make([]string, 0, len(scores))
	for m := range scores {
		members = append(members, m)
	}
	sort.Strings(members)
	next, window := scanWindow(members, cursor, count)
	out := make([]ZMember, 0, len(window))
	for _, m := range window {
		if match == "" || matchPattern(match, m) {
			out = append(out, ZMember{Member: m, Score: scores[m]})
		}
	}
	return next, out, nil
}

// scanWindow returns up to count entries of a sorted slice starting at cursor,
// plus the next cursor (0 once exhausted). An out-of-range cursor terminates at
// 0 instead of panicking. Filtering is left to the caller, so a page may come
// back empty with a non-zero cursor.
func scanWindow(sorted []string, cursor uint64, count int) (uint64, []string) {
	if count <= 0 {
		count = defaultScanCount
	}
	if cursor >= uint64(len(sorted)) {
		return 0, nil
	}
	start := int(cursor)
	// Compare against the remaining tail instead of computing start+count, which
	// would overflow to a negative slice bound on a huge COUNT and panic.
	if count >= len(sorted)-start {
		return 0, sorted[start:]
	}
	end := start + count
	return uint64(end), sorted[start:end]
}
