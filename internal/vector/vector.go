// Package vector implements a flat (brute-force) vector index with cosine
// similarity. It is deliberately simple: V1 trades query speed for zero
// dependencies and predictable behaviour. An HNSW index is a documented future
// upgrade (see PRD §7) that can replace Collection without changing callers.
package vector

import (
	"container/heap"
	"errors"
	"math"
)

// ErrDimMismatch is returned when a vector's length does not match the
// collection's established dimension.
var ErrDimMismatch = errors.New("vector dimension mismatch")

// Item is one stored vector, its id, and optional metadata (used by the
// semantic cache to stash the cached response).
type Item struct {
	ID         string
	Vec        []float32
	Meta       string
	ExpireUnix int64   // Unix seconds the item expires at; 0 == no expiry
	norm       float64 // Euclidean norm of Vec, cached at insert time; not persisted
	// lastAccess orders items for LRU eviction; the store stamps it with
	// UnixNano so back-to-back accesses in the same second keep their order.
	// Not persisted: items loaded from a snapshot start at 0 and rank oldest
	// until they are used again.
	lastAccess int64
}

// expired reports whether the item's deadline has passed at nowUnix. A zero
// deadline never expires, and a zero nowUnix disables expiry checking.
func (it *Item) expired(nowUnix int64) bool {
	return it.ExpireUnix != 0 && nowUnix > it.ExpireUnix
}

// Collection is a flat index of vectors that all share one dimension.
type Collection struct {
	Dim   int
	items map[string]*Item // id -> item
}

// NewCollection returns an empty collection. The dimension is fixed by the
// first vector added.
func NewCollection() *Collection {
	return &Collection{items: map[string]*Item{}}
}

// Set inserts or replaces the vector stored under id. The first vector added
// to an empty collection fixes its dimension; later vectors must match.
// expireUnix is the Unix second the item expires at, 0 for no expiry.
func (c *Collection) Set(id string, vec []float32, meta string, expireUnix int64) error {
	if c.Dim == 0 {
		c.Dim = len(vec)
	} else if len(vec) != c.Dim {
		return ErrDimMismatch
	}
	cp := make([]float32, len(vec))
	copy(cp, vec)
	c.items[id] = &Item{ID: id, Vec: cp, Meta: meta, ExpireUnix: expireUnix, norm: norm(cp)}
	return nil
}

// Touch records that the item was just used, so LRU eviction keeps it longer.
// now is any monotonically increasing ordering value; the store passes
// UnixNano.
func (c *Collection) Touch(id string, now int64) {
	if it, ok := c.items[id]; ok {
		it.lastAccess = now
	}
}

// SweepExpired removes every item whose deadline is past at nowUnix and
// returns how many were removed. Items with no expiry are untouched.
func (c *Collection) SweepExpired(nowUnix int64) int {
	removed := 0
	for id, it := range c.items {
		if it.expired(nowUnix) {
			delete(c.items, id)
			removed++
		}
	}
	return removed
}

// norm returns the Euclidean norm (magnitude) of v.
func norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		f := float64(x)
		sum += f * f
	}
	return math.Sqrt(sum)
}

// evictSample is how many candidates an eviction round examines. Like Redis's
// approximated LRU, evicting the oldest of a small random sample gets close to
// true LRU without keeping the items in an ordered structure.
const evictSample = 5

// EvictLRU removes items until the collection holds at most max, picking each
// victim as the least-recently-used of a sample (map iteration order supplies
// the randomness). It returns how many were evicted. max <= 0 evicts nothing.
func (c *Collection) EvictLRU(max int) int {
	if max <= 0 {
		return 0
	}
	evicted := 0
	for len(c.items) > max {
		var victim *Item
		seen := 0
		for _, it := range c.items {
			if victim == nil || it.lastAccess < victim.lastAccess {
				victim = it
			}
			seen++
			if seen >= evictSample {
				break
			}
		}
		delete(c.items, victim.ID)
		evicted++
	}
	return evicted
}

// Del removes id; it reports whether the id existed.
func (c *Collection) Del(id string) bool {
	if _, ok := c.items[id]; ok {
		delete(c.items, id)
		return true
	}
	return false
}

// Len returns the number of stored vectors.
func (c *Collection) Len() int { return len(c.items) }

// Result is one search hit: an item paired with its cosine similarity score in
// the range [-1, 1] (1 == identical direction).
type Result struct {
	Item  *Item
	Score float64
}

// Search returns the top-k items by cosine similarity to query, highest first.
// Items whose expiry is past at nowUnix are skipped, so an expired entry can
// never win a slot over a live one; nowUnix 0 disables the check. Skipped items
// stay in the collection — reclaiming them is the sweeper's job.
func (c *Collection) Search(query []float32, k int, nowUnix int64) ([]Result, error) {
	if c.Dim != 0 && len(query) != c.Dim {
		return nil, ErrDimMismatch
	}
	n := len(c.items)
	if n == 0 {
		return []Result{}, nil
	}
	// k <= 0 or a k past the collection size both mean "return everything".
	if k <= 0 || k > n {
		k = n
	}
	qn := norm(query)
	// Keep the k highest scores in a size-k min-heap: its root is the weakest of
	// the current best, so evicting it is cheap. This is O(n log k) with no
	// allocation proportional to the whole collection.
	h := make(topK, 0, k)
	for _, it := range c.items {
		if it.expired(nowUnix) {
			continue
		}
		s := cosineNorm(query, qn, it)
		if len(h) < k {
			heap.Push(&h, Result{Item: it, Score: s})
		} else if s > h[0].Score {
			h[0] = Result{Item: it, Score: s}
			heap.Fix(&h, 0)
		}
	}
	// Drain the heap into descending score order by popping the weakest to the
	// tail each time.
	out := make([]Result, len(h))
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(&h).(Result)
	}
	return out, nil
}

// topK is a min-heap of results ordered by ascending score, so the root is the
// weakest of the current top-k candidates and the first to be evicted.
type topK []Result

func (h topK) Len() int           { return len(h) }
func (h topK) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h topK) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *topK) Push(x any) { *h = append(*h, x.(Result)) }

func (h *topK) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

// cosineNorm is cosine similarity between query (with precomputed norm qn) and a
// stored item whose norm is already cached. It only does the dot product in the
// loop; the magnitudes are supplied, not recomputed. A zero-magnitude vector on
// either side yields 0, matching the flat cosine helper.
func cosineNorm(query []float32, qn float64, it *Item) float64 {
	if qn == 0 || it.norm == 0 || len(query) != len(it.Vec) {
		return 0
	}
	var dot float64
	for i := range query {
		dot += float64(query[i]) * float64(it.Vec[i])
	}
	return dot / (qn * it.norm)
}

// Items returns all stored items (used for snapshotting). The slice is freshly
// allocated; callers may not mutate the underlying vectors.
func (c *Collection) Items() []*Item {
	out := make([]*Item, 0, len(c.items))
	for _, it := range c.items {
		out = append(out, it)
	}
	return out
}
