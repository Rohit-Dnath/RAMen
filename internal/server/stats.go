package server

import "sync/atomic"

// Stats holds server-wide counters surfaced by INFO and the dashboard.
type Stats struct {
	Commands    atomic.Int64
	Connections atomic.Int64 // currently open connections
	TotalConns  atomic.Int64 // connections accepted since start
	CacheHits   atomic.Int64 // semantic cache hits
	CacheMisses atomic.Int64 // semantic cache misses
}

// HitRatio returns the semantic-cache hit ratio in [0,1]; 0 when there have
// been no lookups yet.
func (s *Stats) HitRatio() float64 {
	h := s.CacheHits.Load()
	m := s.CacheMisses.Load()
	total := h + m
	if total == 0 {
		return 0
	}
	return float64(h) / float64(total)
}
