// Package store is RAMen's in-memory keyspace. Keys are spread across a fixed
// number of shards, each guarded by its own RWMutex, so unrelated keys rarely
// contend for the same lock. Expiry is handled both lazily (on access) and by
// a background sweep (see expiry.go). The PRD calls for a "sharded in-process
// map" and to "revisit only if benchmarks show real lock contention" (§9).
package store

import (
	"hash/fnv"
	"math/rand"
	"sync"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/vector"
)

const shardCount = 256

// value holds any of the supported data types. The concrete Go types are:
//
//	string                 -> string
//	hash                   -> map[string]string
//	list                   -> *list   (see list.go)
//	set                    -> map[string]struct{}
//	sorted set             -> *zset    (see zset.go)
//	vector collection      -> *vector.Collection
type value = any

type entry struct {
	val      value
	expireAt time.Time // zero == no expiry
}

func (e *entry) expired(now time.Time) bool {
	return !e.expireAt.IsZero() && now.After(e.expireAt)
}

type shard struct {
	mu sync.RWMutex
	m  map[string]*entry
}

// Store is the full keyspace, safe for concurrent use.
type Store struct {
	shards [shardCount]*shard
	now    func() time.Time // injectable clock for tests
}

// New returns an empty store.
func New() *Store {
	s := &Store{now: time.Now}
	for i := range s.shards {
		s.shards[i] = &shard{m: make(map[string]*entry)}
	}
	return s
}

func (s *Store) shardFor(key string) *shard {
	return s.shards[s.shardIndex(key)]
}

// shardIndex returns the index of the shard that owns key. Rename needs the
// index, not just the shard, so it can lock two shards in a fixed order.
func (s *Store) shardIndex(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % shardCount)
}

// getLive returns the live entry for key, deleting it first if it has expired
// (lazy expiry). Because it mutates the shard map, the caller MUST hold the
// shard write lock (sh.mu.Lock). Read paths holding only a read lock must use
// peekLive instead.
func (sh *shard) getLive(key string, now time.Time) (*entry, bool) {
	e, ok := sh.m[key]
	if !ok {
		return nil, false
	}
	if e.expired(now) {
		delete(sh.m, key)
		return nil, false
	}
	return e, true
}

// peekLive returns the live entry for key without mutating the shard map: an
// expired entry is reported as absent but left in place for the background
// sweeper (or a later write) to reclaim. It is safe to call while holding only
// the shard read lock (sh.mu.RLock).
func (sh *shard) peekLive(key string, now time.Time) (*entry, bool) {
	e, ok := sh.m[key]
	if !ok || e.expired(now) {
		return nil, false
	}
	return e, true
}

// Exists reports how many of the given keys currently exist.
func (s *Store) Exists(keys ...string) int {
	now := s.now()
	count := 0
	for _, k := range keys {
		sh := s.shardFor(k)
		sh.mu.Lock()
		if _, ok := sh.getLive(k, now); ok {
			count++
		}
		sh.mu.Unlock()
	}
	return count
}

// Del removes the given keys, returning the number actually deleted.
func (s *Store) Del(keys ...string) int {
	now := s.now()
	count := 0
	for _, k := range keys {
		sh := s.shardFor(k)
		sh.mu.Lock()
		if _, ok := sh.getLive(k, now); ok {
			delete(sh.m, k)
			count++
		}
		sh.mu.Unlock()
	}
	return count
}

// Type returns the Redis type name of key ("string", "hash", "list", "set",
// "zset", "vector") or "none" if it does not exist.
func (s *Store) Type(key string) string {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.getLive(key, s.now())
	if !ok {
		return "none"
	}
	switch e.val.(type) {
	case string:
		return "string"
	case map[string]string:
		return "hash"
	case *list:
		return "list"
	case map[string]struct{}:
		return "set"
	case *zset:
		return "zset"
	case *vector.Collection:
		return "vector"
	default:
		return "none"
	}
}

// Rename moves src to newkey, overwriting newkey if it already exists. The whole
// entry is moved, so any TTL travels with the key. It returns ErrNoSuchKey when
// src does not exist, matching Redis.
func (s *Store) Rename(src, newkey string) error {
	_, err := s.rename(src, newkey, false)
	return err
}

// RenameNX renames src to newkey only when newkey does not already exist. It
// reports whether the rename happened (false when newkey exists) and returns
// ErrNoSuchKey when src does not exist.
func (s *Store) RenameNX(src, newkey string) (bool, error) {
	return s.rename(src, newkey, true)
}

// rename is the shared implementation of RENAME/RENAMENX. It locks the source
// and destination shards in a fixed (lowest-index-first) order so two renames
// running between the same pair of shards cannot deadlock, and locks only once
// when both keys live on the same shard.
func (s *Store) rename(src, newkey string, nx bool) (bool, error) {
	si, di := s.shardIndex(src), s.shardIndex(newkey)
	ss, ds := s.shards[si], s.shards[di]

	switch {
	case si == di:
		ss.mu.Lock()
		defer ss.mu.Unlock()
	case si < di:
		ss.mu.Lock()
		defer ss.mu.Unlock()
		ds.mu.Lock()
		defer ds.mu.Unlock()
	default:
		ds.mu.Lock()
		defer ds.mu.Unlock()
		ss.mu.Lock()
		defer ss.mu.Unlock()
	}

	now := s.now()
	e, ok := ss.getLive(src, now)
	if !ok {
		return false, ErrNoSuchKey
	}
	// Renaming a key to itself is a no-op that still succeeds; RENAMENX treats
	// the destination as already present and reports that nothing moved.
	if src == newkey {
		return !nx, nil
	}
	if nx {
		if _, exists := ds.peekLive(newkey, now); exists {
			return false, nil
		}
	}
	ds.m[newkey] = e
	delete(ss.m, src)
	return true, nil
}

// Expire sets a relative TTL in milliseconds on an existing key. It reports
// whether the key existed.
func (s *Store) Expire(key string, ttl time.Duration) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.getLive(key, s.now())
	if !ok {
		return false
	}
	e.expireAt = s.now().Add(ttl)
	return true
}

// ExpireAt sets an absolute expiry deadline on an existing key. A deadline that
// is already in the past deletes the key right away, matching Redis. It reports
// whether the key existed.
func (s *Store) ExpireAt(key string, at time.Time) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.getLive(key, s.now())
	if !ok {
		return false
	}
	if !at.After(s.now()) {
		delete(sh.m, key)
		return true
	}
	e.expireAt = at
	return true
}

// Persist removes any TTL from key, returning whether a TTL was removed.
func (s *Store) Persist(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, ok := sh.getLive(key, s.now())
	if !ok || e.expireAt.IsZero() {
		return false
	}
	e.expireAt = time.Time{}
	return true
}

// TTL returns the remaining time to live for key. ok is false when the key
// does not exist; hasTTL is false when the key exists but is persistent.
func (s *Store) TTL(key string) (d time.Duration, hasTTL, ok bool) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, false, false
	}
	if e.expireAt.IsZero() {
		return 0, false, true
	}
	return time.Until(e.expireAt), true, true
}

// ExpireTime returns the absolute expiry deadline for key. ok is false when the
// key does not exist; hasTTL is false when the key exists but is persistent.
func (s *Store) ExpireTime(key string) (at time.Time, hasTTL, ok bool) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return time.Time{}, false, false
	}
	if e.expireAt.IsZero() {
		return time.Time{}, false, true
	}
	return e.expireAt, true, true
}

// Keys returns every live key matching the glob-style pattern.
func (s *Store) Keys(pattern string) []string {
	now := s.now()
	var out []string
	for _, sh := range s.shards {
		sh.mu.RLock()
		for k, e := range sh.m {
			if e.expired(now) {
				continue
			}
			if matchPattern(pattern, k) {
				out = append(out, k)
			}
		}
		sh.mu.RUnlock()
	}
	return out
}

// DBSize returns the number of live keys across all shards.
func (s *Store) DBSize() int {
	now := s.now()
	n := 0
	for _, sh := range s.shards {
		sh.mu.RLock()
		for _, e := range sh.m {
			if !e.expired(now) {
				n++
			}
		}
		sh.mu.RUnlock()
	}
	return n
}

// RandomKey returns a randomly chosen live key, or ok=false when the keyspace
// is empty. It starts at a random shard and scans forward, returning a key from
// the first shard that holds one; Go's randomised map iteration makes the pick
// within that shard random too. Expired keys are skipped but left in place for
// the sweeper, so only a read lock is taken (never getLive, which deletes and
// would race concurrent readers).
func (s *Store) RandomKey() (string, bool) {
	now := s.now()
	start := rand.Intn(shardCount)
	for i := 0; i < shardCount; i++ {
		sh := s.shards[(start+i)%shardCount]
		sh.mu.RLock()
		for k, e := range sh.m {
			if !e.expired(now) {
				sh.mu.RUnlock()
				return k, true
			}
		}
		sh.mu.RUnlock()
	}
	return "", false
}

// Flush removes every key.
func (s *Store) Flush() {
	for _, sh := range s.shards {
		sh.mu.Lock()
		sh.m = make(map[string]*entry)
		sh.mu.Unlock()
	}
}
