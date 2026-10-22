package store

import (
	"context"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/vector"
)

// StartSweeper runs a background goroutine that periodically scans shards and
// deletes expired keys. Lazy expiry already removes keys on access; the sweep
// reclaims memory for keys that are never read again. It stops when ctx is
// cancelled.
func (s *Store) StartSweeper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

// sweep deletes expired keys from every shard, and expired items from inside
// vector collections. The semantic cache stores per-item TTLs on vectors in
// one collection key that never itself expires, so without the inner sweep an
// entry that expires and never again wins a search would stay forever (#55).
func (s *Store) sweep() {
	now := s.now()
	nowUnix := now.Unix()
	for _, sh := range s.shards {
		sh.mu.Lock()
		for k, e := range sh.m {
			if e.expired(now) {
				delete(sh.m, k)
				continue
			}
			if c, ok := e.val.(*vector.Collection); ok {
				c.SweepExpired(nowUnix)
				if c.Len() == 0 {
					// Match VDel: a collection emptied of vectors drops the key.
					delete(sh.m, k)
				}
			}
		}
		sh.mu.Unlock()
	}
}
