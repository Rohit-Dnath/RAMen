package store

import (
	"errors"
	"math"
	"sort"
)

// ErrScoreNaN mirrors Redis' ZINCRBY error when an increment would make the
// resulting score NaN (e.g. +inf plus -inf).
var ErrScoreNaN = errors.New("ERR resulting score is not a number (NaN)")

// zset is a sorted set: a member->score map. V1 sorts on demand rather than
// maintaining a skiplist, which is adequate for the basic range ops the PRD
// scopes (ZADD/ZRANGE/ZRANGEBYSCORE/ZSCORE/ZCARD).
type zset struct {
	scores map[string]float64
}

func asZSet(e *entry) (*zset, error) {
	z, ok := e.val.(*zset)
	if !ok {
		return nil, ErrWrongType
	}
	return z, nil
}

// ZMember pairs a member with its score.
type ZMember struct {
	Member string
	Score  float64
}

// ZAdd sets the score for each member (creating the set if absent) and returns
// the number of newly added members.
func (s *Store) ZAdd(key string, members []ZMember) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var z *zset
	if found {
		var err error
		if z, err = asZSet(e); err != nil {
			return 0, err
		}
	} else {
		z = &zset{scores: make(map[string]float64)}
		sh.m[key] = &entry{val: z}
	}
	added := 0
	for _, m := range members {
		if _, ok := z.scores[m.Member]; !ok {
			added++
		}
		z.scores[m.Member] = m.Score
	}
	return added, nil
}

// ZIncrBy adds increment to member's score (treating a missing member as 0,
// creating the set if absent) and returns the new score. It returns ErrScoreNaN
// if the result would be NaN, leaving the set untouched.
func (s *Store) ZIncrBy(key, member string, increment float64) (float64, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var z *zset
	if found {
		var err error
		if z, err = asZSet(e); err != nil {
			return 0, err
		}
	}
	var cur float64
	if z != nil {
		cur = z.scores[member] // 0 if the member is absent
	}
	newScore := cur + increment
	if math.IsNaN(newScore) {
		return 0, ErrScoreNaN
	}
	if z == nil {
		z = &zset{scores: make(map[string]float64)}
		sh.m[key] = &entry{val: z}
	}
	z.scores[member] = newScore
	return newScore, nil
}

// ZRem removes members and returns how many were removed.
func (s *Store) ZRem(key string, members ...string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return 0, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, m := range members {
		if _, ok := z.scores[m]; ok {
			delete(z.scores, m)
			removed++
		}
	}
	if len(z.scores) == 0 {
		delete(sh.m, key)
	}
	return removed, nil
}

// ZScore returns the score of member.
func (s *Store) ZScore(key, member string) (float64, bool, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, false, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return 0, false, err
	}
	sc, ok := z.scores[member]
	return sc, ok, nil
}

// ZCard returns the number of members.
func (s *Store) ZCard(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return 0, err
	}
	return len(z.scores), nil
}

// zrankOf returns the ascending 0-based rank of member and the set size, or
// found=false when the key or member is missing.
func (s *Store) zrankOf(key, member string) (rank, size int, found bool, err error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, ok := sh.peekLive(key, s.now())
	if !ok {
		return 0, 0, false, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return 0, 0, false, err
	}
	if _, ok := z.scores[member]; !ok {
		return 0, 0, false, nil
	}
	sorted := z.sortedMembers()
	for i, m := range sorted {
		if m.Member == member {
			return i, len(sorted), true, nil
		}
	}
	return 0, 0, false, nil
}

// ZRank returns the ascending 0-based rank of member; found is false when the
// key or member is missing.
func (s *Store) ZRank(key, member string) (int, bool, error) {
	rank, _, found, err := s.zrankOf(key, member)
	return rank, found, err
}

// ZRevRank returns the descending 0-based rank of member; found is false when
// the key or member is missing.
func (s *Store) ZRevRank(key, member string) (int, bool, error) {
	rank, size, found, err := s.zrankOf(key, member)
	if !found {
		return 0, false, err
	}
	return size - 1 - rank, true, nil
}

// sortedMembers returns all members ordered by score ascending, ties broken
// lexicographically (Redis ordering).
func (z *zset) sortedMembers() []ZMember {
	out := make([]ZMember, 0, len(z.scores))
	for m, sc := range z.scores {
		out = append(out, ZMember{Member: m, Score: sc})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Member < out[j].Member
	})
	return out
}

// ZRange returns members in the inclusive rank range [start, stop] ordered by
// score (negative indices count from the end).
func (s *Store) ZRange(key string, start, stop int) ([]ZMember, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return nil, err
	}
	sorted := z.sortedMembers()
	start, stop = normalizeRange(start, stop, len(sorted))
	if start > stop {
		return []ZMember{}, nil
	}
	return sorted[start : stop+1], nil
}

// ZRevRange returns members in the inclusive rank range [start, stop] ordered by
// score DESCENDING (negative indices count from the end).
func (s *Store) ZRevRange(key string, start, stop int) ([]ZMember, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return nil, err
	}
	sorted := z.sortedMembers()
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}
	start, stop = normalizeRange(start, stop, len(sorted))
	if start > stop {
		return []ZMember{}, nil
	}
	return sorted[start : stop+1], nil
}

// ZRangeByScore returns members whose score lies in [min, max] inclusive,
// ordered by score.
func (s *Store) ZRangeByScore(key string, min, max float64) ([]ZMember, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return nil, err
	}
	var out []ZMember
	for _, m := range z.sortedMembers() {
		if m.Score >= min && m.Score <= max {
			out = append(out, m)
		}
	}
	return out, nil
}

// ZCount returns how many members have a score within the given bounds. minExcl
// and maxExcl make the corresponding bound exclusive rather than inclusive.
func (s *Store) ZCount(key string, min float64, minExcl bool, max float64, maxExcl bool) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	z, err := asZSet(e)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, sc := range z.scores {
		aboveMin := sc >= min
		if minExcl {
			aboveMin = sc > min
		}
		belowMax := sc <= max
		if maxExcl {
			belowMax = sc < max
		}
		if aboveMin && belowMax {
			count++
		}
	}
	return count, nil
}
