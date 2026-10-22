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

func asSet(e *entry) (map[string]struct{}, error) {
	st, ok := e.val.(map[string]struct{})
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
	var set map[string]struct{}
	if found {
		var err error
		if set, err = asSet(e); err != nil {
			return 0, err
		}
	} else {
		set = make(map[string]struct{})
		sh.m[key] = &entry{val: set}
	}
	added := 0
	for _, m := range members {
		if _, ok := set[m]; !ok {
			set[m] = struct{}{}
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
	set, err := asSet(e)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, m := range members {
		if _, ok := set[m]; ok {
			delete(set, m)
			removed++
		}
	}
	if len(set) == 0 {
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
	set, err := asSet(e)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	return out, nil
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
	set, err := asSet(e)
	if err != nil {
		return false, err
	}
	_, ok := set[member]
	return ok, nil
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
	set, err := asSet(e)
	if err != nil {
		return 0, err
	}
	return len(set), nil
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
		set, err := asSet(e)
		if err != nil {
			sh.mu.RUnlock()
			return nil, err
		}
		cp := make(map[string]struct{}, len(set))
		for m := range set {
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

// sampleMembers picks members per Redis SPOP/SRANDMEMBER count rules: count >= 0
// yields that many DISTINCT members (at most all of them); count < 0 yields
// exactly -count members WITH repetition. members may be reordered in place.
func sampleMembers(members []string, count int64) []string {
	n := len(members)
	if n == 0 || count == 0 {
		return []string{}
	}
	if count > 0 {
		if count >= int64(n) {
			return append([]string(nil), members...)
		}
		rand.Shuffle(n, func(i, j int) { members[i], members[j] = members[j], members[i] })
		return append([]string(nil), members[:count]...)
	}
	// count < 0: with repetition. The caller bounds the magnitude, so -count is
	// safe to convert and cannot overflow.
	reps := int(-count)
	out := make([]string, reps)
	for i := range out {
		out[i] = members[rand.Intn(n)]
	}
	return out
}

// SPop removes and returns up to count random distinct members. The key is
// deleted once emptied. count must be >= 0 (the caller rejects negatives).
func (s *Store) SPop(key string, count int64) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return nil, nil
	}
	set, err := asSet(e)
	if err != nil {
		return nil, err
	}
	members := make([]string, 0, len(set))
	for m := range set {
		members = append(members, m)
	}
	picked := sampleMembers(members, count)
	for _, m := range picked {
		delete(set, m)
	}
	if len(set) == 0 {
		delete(sh.m, key)
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
	set, err := asSet(e)
	if err != nil {
		return nil, err
	}
	// Apply the with-repetition cap only once the key is known to hold a set,
	// so a missing key still returns empty and a wrong type still WRONGTYPEs.
	if count < -maxSampleReps {
		return nil, ErrSampleCount
	}
	members := make([]string, 0, len(set))
	for m := range set {
		members = append(members, m)
	}
	return sampleMembers(members, count), nil
}
