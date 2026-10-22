package store

import (
	"errors"
	"math/rand"
)

// ErrSampleCount mirrors Redis' out-of-range error, returned when SRANDMEMBER's
// with-repetition count is too large to serve safely.
var ErrSampleCount = errors.New("ERR value is out of range")

// maxSampleReps caps how many members SRANDMEMBER returns for a negative
// (with-repetition) count, guarding against an unbounded allocation.
const maxSampleReps = 1 << 20

// set holds its members in a slice, with idx mapping each member to its
// position, so SPOP and SRANDMEMBER can pick a uniformly random member in O(1)
// by index; a plain map can only be sampled by copying it. Removal moves the
// last member into the freed slot, keeping add, remove and lookup O(1).
type set struct {
	idx   map[string]int
	items []string
}

func newSet(capacity int) *set {
	return &set{idx: make(map[string]int, capacity), items: make([]string, 0, capacity)}
}

func (st *set) len() int { return len(st.items) }

func (st *set) has(m string) bool {
	_, ok := st.idx[m]
	return ok
}

func (st *set) add(m string) bool {
	if _, ok := st.idx[m]; ok {
		return false
	}
	st.idx[m] = len(st.items)
	st.items = append(st.items, m)
	return true
}

func (st *set) remove(m string) bool {
	i, ok := st.idx[m]
	if !ok {
		return false
	}
	last := len(st.items) - 1
	if i != last {
		moved := st.items[last]
		st.items[i] = moved
		st.idx[moved] = i
	}
	st.items[last] = "" // drop the reference so the string can be collected
	st.items = st.items[:last]
	delete(st.idx, m)
	return true
}

func asSet(e *entry) (*set, error) {
	st, ok := e.val.(*set)
	if !ok {
		return nil, ErrWrongType
	}
	return st, nil
}

// SAdd adds members to the set at key (creating it if absent) and returns how
// many were newly added.
func (s *Store) SAdd(key string, members ...string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var st *set
	if found {
		var err error
		if st, err = asSet(e); err != nil {
			return 0, err
		}
	} else {
		st = newSet(len(members))
		sh.m[key] = &entry{val: st}
	}
	added := 0
	for _, m := range members {
		if st.add(m) {
			added++
		}
	}
	return added, nil
}

// SRem removes members and returns how many were removed; the key is dropped
// when emptied.
func (s *Store) SRem(key string, members ...string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return 0, nil
	}
	st, err := asSet(e)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, m := range members {
		if st.remove(m) {
			removed++
		}
	}
	if st.len() == 0 {
		delete(sh.m, key)
	}
	return removed, nil
}

// SMembers returns all members of the set.
func (s *Store) SMembers(key string) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	st, err := asSet(e)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), st.items...), nil
}

// SIsMember reports whether member is in the set.
func (s *Store) SIsMember(key, member string) (bool, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return false, nil
	}
	st, err := asSet(e)
	if err != nil {
		return false, err
	}
	return st.has(member), nil
}

// SCard returns the set cardinality.
func (s *Store) SCard(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	st, err := asSet(e)
	if err != nil {
		return 0, err
	}
	return st.len(), nil
}

// readSets returns a private copy of each key's set, or ErrWrongType if any key
// holds a non-set. A missing key yields a nil (empty) set, but every key is
// still type-checked — Redis inspects all keys before deciding the result, so
// a wrong-typed key errors even when an earlier key is missing. Copying under
// each shard's read lock lets the multi-key algebra run afterwards without
// holding any lock, matching the per-key read style used elsewhere (e.g. MGET).
func (s *Store) readSets(keys []string) ([]map[string]struct{}, error) {
	now := s.now()
	out := make([]map[string]struct{}, len(keys))
	for i, k := range keys {
		sh := s.shardFor(k)
		sh.mu.RLock()
		e, found := sh.peekLive(k, now)
		if !found {
			sh.mu.RUnlock()
			continue // leave out[i] nil, i.e. an empty set
		}
		st, err := asSet(e)
		if err != nil {
			sh.mu.RUnlock()
			return nil, err
		}
		cp := make(map[string]struct{}, st.len())
		for _, m := range st.items {
			cp[m] = struct{}{}
		}
		out[i] = cp
		sh.mu.RUnlock()
	}
	return out, nil
}

// SInter returns the members present in every set at keys. A missing key is an
// empty set, so the result is empty. Returns ErrWrongType if any key is not a
// set, regardless of where it sits relative to a missing key.
func (s *Store) SInter(keys []string) ([]string, error) {
	sets, err := s.readSets(keys)
	if err != nil {
		return nil, err
	}
	// Any empty (or missing) set makes the intersection empty; otherwise scan
	// the smallest set and keep members found in all the others.
	base := sets[0]
	for _, st := range sets {
		if len(st) == 0 {
			return nil, nil
		}
		if len(st) < len(base) {
			base = st
		}
	}
	out := []string{}
	for m := range base {
		inAll := true
		for _, st := range sets {
			if _, ok := st[m]; !ok {
				inAll = false
				break
			}
		}
		if inAll {
			out = append(out, m)
		}
	}
	return out, nil
}

// SUnion returns the distinct members across all sets at keys (missing keys are
// empty). Returns ErrWrongType if any key is not a set.
func (s *Store) SUnion(keys []string) ([]string, error) {
	sets, err := s.readSets(keys)
	if err != nil {
		return nil, err
	}
	u := make(map[string]struct{})
	for _, st := range sets {
		for m := range st {
			u[m] = struct{}{}
		}
	}
	out := make([]string, 0, len(u))
	for m := range u {
		out = append(out, m)
	}
	return out, nil
}

// SDiff returns the members of the first set that appear in none of the rest
// (missing keys are empty). Returns ErrWrongType if any key is not a set.
func (s *Store) SDiff(keys []string) ([]string, error) {
	sets, err := s.readSets(keys)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for m := range sets[0] {
		inOthers := false
		for _, st := range sets[1:] {
			if _, ok := st[m]; ok {
				inOthers = true
				break
			}
		}
		if !inOthers {
			out = append(out, m)
		}
	}
	return out, nil
}

// sampleDistinct returns count (0 < count < len(items)) distinct members chosen
// uniformly at random, without modifying items, in O(count) time: a small
// sample draws random indexes and skips repeats, while a sample of more than
// half the set (whose output is O(n) anyway) shuffles the first count slots of
// a copy.
func sampleDistinct(items []string, count int) []string {
	n := len(items)
	out := make([]string, 0, count)
	if count <= n/2 {
		// each draw repeats with probability < 1/2, so this takes < 2*count draws on average
		seen := make(map[int]struct{}, count)
		for len(out) < count {
			i := rand.Intn(n)
			if _, dup := seen[i]; dup {
				continue
			}
			seen[i] = struct{}{}
			out = append(out, items[i])
		}
		return out
	}
	cp := append([]string(nil), items...)
	for i := 0; i < count; i++ {
		j := i + rand.Intn(n-i)
		cp[i], cp[j] = cp[j], cp[i]
	}
	return append(out, cp[:count]...)
}

// SPop removes and returns up to count random distinct members. The key is
// deleted once emptied. count must be >= 0 (the caller rejects negatives).
// Each pick is O(1), so popping count members costs O(count), not O(set size).
func (s *Store) SPop(key string, count int64) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return nil, nil
	}
	st, err := asSet(e)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return []string{}, nil
	}
	if count >= int64(st.len()) {
		delete(sh.m, key)
		return st.items, nil // the set is gone, so its slice can be handed over
	}
	picked := make([]string, count)
	for i := range picked {
		m := st.items[rand.Intn(st.len())]
		st.remove(m)
		picked[i] = m
	}
	return picked, nil
}

// SRandMember returns up to count random members without modifying the set:
// count >= 0 yields distinct members (at most the set size); count < 0 yields
// exactly -count members with repetition. It returns ErrSampleCount when a
// negative count's magnitude exceeds the safe limit.
func (s *Store) SRandMember(key string, count int64) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	st, err := asSet(e)
	if err != nil {
		return nil, err
	}
	// Apply the with-repetition cap only once the key is known to hold a set,
	// so a missing key still returns empty and a wrong type still WRONGTYPEs.
	if count < -maxSampleReps {
		return nil, ErrSampleCount
	}
	n := st.len()
	switch {
	case count == 0:
		return []string{}, nil
	case count < 0:
		// with repetition; the cap above makes -count safe to convert
		out := make([]string, -count)
		for i := range out {
			out[i] = st.items[rand.Intn(n)]
		}
		return out, nil
	case count >= int64(n):
		return append([]string(nil), st.items...), nil
	default:
		return sampleDistinct(st.items, int(count)), nil
	}
}
