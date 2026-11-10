package vector

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"
)

func TestSetSearch(t *testing.T) {
	c := NewCollection()
	if err := c.Set("a", []float32{1, 0, 0}, "alpha", 0); err != nil {
		t.Fatal(err)
	}
	c.Set("b", []float32{0, 1, 0}, "beta", 0)
	c.Set("c", []float32{0.9, 0.1, 0}, "gamma", 0)

	res, err := c.Search([]float32{1, 0, 0}, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	if res[0].Item.ID != "a" {
		t.Fatalf("nearest should be a, got %s", res[0].Item.ID)
	}
	if res[0].Score < 0.99 {
		t.Fatalf("identical vector should score ~1, got %f", res[0].Score)
	}
}

func TestDimMismatch(t *testing.T) {
	c := NewCollection()
	c.Set("a", []float32{1, 2, 3}, "", 0)
	if err := c.Set("b", []float32{1, 2}, "", 0); err != ErrDimMismatch {
		t.Fatalf("want ErrDimMismatch, got %v", err)
	}
}

// bruteCosine recomputes cosine similarity from scratch, independent of the
// collection's cached norms, so it can serve as the ranking oracle.
func bruteCosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TestSearchMatchesBruteForce proves the min-heap selection and cached-norm
// scoring return exactly the same top-k, in the same order, as a from-scratch
// cosine ranking. The optimisation must cost zero recall.
func TestSearchMatchesBruteForce(t *testing.T) {
	const n, dim, k = 500, 64, 10
	c, query := randVectors(n, dim)

	got, err := c.Search(query, k, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != k {
		t.Fatalf("got %d results, want %d", len(got), k)
	}

	type scored struct {
		id    string
		score float64
	}
	oracle := make([]scored, 0, n)
	for _, it := range c.Items() {
		oracle = append(oracle, scored{it.ID, bruteCosine(query, it.Vec)})
	}
	sort.Slice(oracle, func(i, j int) bool { return oracle[i].score > oracle[j].score })

	for i := 0; i < k; i++ {
		if got[i].Item.ID != oracle[i].id {
			t.Fatalf("rank %d: got %q (%.6f), want %q (%.6f)", i, got[i].Item.ID, got[i].Score, oracle[i].id, oracle[i].score)
		}
		if math.Abs(got[i].Score-oracle[i].score) > 1e-9 {
			t.Fatalf("rank %d score %.12f vs oracle %.12f", i, got[i].Score, oracle[i].score)
		}
		if i > 0 && got[i].Score > got[i-1].Score {
			t.Fatalf("results not in descending order at rank %d", i)
		}
	}
}

// TestSearchEdgeCases exercises the boundary conditions of Search: empty
// collection, k values at and beyond the collection size, k <= 0, a
// zero-magnitude query, and a replace updating the cached norm.
func TestSearchEdgeCases(t *testing.T) {
	// Empty collection: no dimension, any query, empty (non-nil) result.
	empty := NewCollection()
	if res, err := empty.Search([]float32{1, 2, 3}, 5, 0); err != nil || res == nil || len(res) != 0 {
		t.Fatalf("empty collection Search = (%v, %v)", res, err)
	}

	c := NewCollection()
	c.Set("a", []float32{1, 0}, "", 0)
	c.Set("b", []float32{0, 1}, "", 0)
	c.Set("d", []float32{1, 1}, "", 0)

	// k larger than the collection returns everything, still sorted.
	if res, _ := c.Search([]float32{1, 0}, 99, 0); len(res) != 3 {
		t.Fatalf("k>n returned %d results, want 3", len(res))
	}
	// k == n returns everything.
	if res, _ := c.Search([]float32{1, 0}, 3, 0); len(res) != 3 {
		t.Fatalf("k==n returned %d results, want 3", len(res))
	}
	// k <= 0 means "return everything".
	for _, k := range []int{0, -1} {
		if res, _ := c.Search([]float32{1, 0}, k, 0); len(res) != 3 {
			t.Fatalf("k=%d returned %d results, want 3", k, len(res))
		}
	}
	// Descending order holds for a normal top-k.
	res, _ := c.Search([]float32{1, 0}, 2, 0)
	if len(res) != 2 || res[0].Score < res[1].Score {
		t.Fatalf("top-2 not descending: %v", res)
	}

	// A zero-magnitude query scores every item 0 and must not panic or divide by
	// zero; it still returns k items.
	if z, _ := c.Search([]float32{0, 0}, 2, 0); len(z) != 2 {
		t.Fatalf("zero-query returned %d results, want 2", len(z))
	} else if z[0].Score != 0 {
		t.Fatalf("zero-query score = %v, want 0", z[0].Score)
	}

	// Dimension mismatch is rejected.
	if _, err := c.Search([]float32{1, 2, 3}, 1, 0); err != ErrDimMismatch {
		t.Fatalf("dim mismatch err = %v", err)
	}

	// Replacing an id must refresh its cached norm: after shrinking a's vector,
	// its cosine to the query is recomputed correctly (identical direction -> ~1).
	c.Set("a", []float32{5, 0}, "", 0) // same direction, different magnitude
	if r, _ := c.Search([]float32{1, 0}, 1, 0); r[0].Item.ID != "a" || r[0].Score < 0.99 {
		t.Fatalf("replace did not refresh norm: %v", r)
	}
}

// randVectors builds a collection of n random vectors of the given dimension,
// using a fixed seed so the benchmark is reproducible.
func randVectors(n, dim int) (*Collection, []float32) {
	rng := rand.New(rand.NewSource(1))
	c := NewCollection()
	for i := 0; i < n; i++ {
		v := make([]float32, dim)
		for j := range v {
			v[j] = rng.Float32()
		}
		c.Set(strconv.Itoa(i), v, "", 0)
	}
	query := make([]float32, dim)
	for j := range query {
		query[j] = rng.Float32()
	}
	return c, query
}

// BenchmarkVSearch measures a top-10 search over collections of 1k/10k/100k
// vectors at 768 dimensions (a common embedding size).
func BenchmarkVSearch(b *testing.B) {
	const dim = 768
	for _, n := range []int{1000, 10000, 100000} {
		c, query := randVectors(n, dim)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := c.Search(query, 10, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestSearchSkipsExpired(t *testing.T) {
	c := NewCollection()
	c.Set("live", []float32{1, 0}, "", 0)
	c.Set("dying", []float32{9, 1}, "", 100)
	q := []float32{9, 1}

	// At the deadline the item still matches (expiry is strictly after it).
	if res, _ := c.Search(q, 1, 100); len(res) != 1 || res[0].Item.ID != "dying" {
		t.Fatalf("Search at the deadline = %v, want dying", res)
	}
	// Past the deadline it is invisible and the live item wins the slot.
	if res, _ := c.Search(q, 1, 101); len(res) != 1 || res[0].Item.ID != "live" {
		t.Fatalf("Search past the deadline = %v, want live", res)
	}
	// A zero nowUnix disables the check entirely.
	if res, _ := c.Search(q, 1, 0); len(res) != 1 || res[0].Item.ID != "dying" {
		t.Fatalf("Search with expiry disabled = %v, want dying", res)
	}
	// Replacing an item refreshes its deadline.
	c.Set("dying", []float32{9, 1}, "", 0)
	if res, _ := c.Search(q, 1, 101); len(res) != 1 || res[0].Item.ID != "dying" {
		t.Fatalf("Search after refresh = %v, want dying", res)
	}
}

func TestSweepExpired(t *testing.T) {
	c := NewCollection()
	c.Set("a", []float32{1, 0}, "", 0)
	c.Set("b", []float32{0, 1}, "", 100)
	c.Set("d", []float32{1, 1}, "", 50)

	if n := c.SweepExpired(50); n != 0 {
		t.Fatalf("SweepExpired at the deadline removed %d, want 0", n)
	}
	if n := c.SweepExpired(51); n != 1 || c.Len() != 2 {
		t.Fatalf("SweepExpired(51) = %d removed, len %d; want 1 and 2", n, c.Len())
	}
	if n := c.SweepExpired(1000); n != 1 || c.Len() != 1 {
		t.Fatalf("SweepExpired(1000) = %d removed, len %d; want 1 and 1", n, c.Len())
	}
	if _, ok := c.items["a"]; !ok {
		t.Fatal("SweepExpired removed the item with no expiry")
	}
}

func TestEvictLRU(t *testing.T) {
	c := NewCollection()
	c.Set("a", []float32{1, 0}, "", 0)
	c.Set("b", []float32{0, 1}, "", 0)
	c.Set("d", []float32{1, 1}, "", 0)
	c.Touch("a", 30)
	c.Touch("b", 10)
	c.Touch("d", 20)

	// max <= 0 means unbounded: nothing is evicted.
	if n := c.EvictLRU(0); n != 0 || c.Len() != 3 {
		t.Fatalf("EvictLRU(0) evicted %d, len %d; want none", n, c.Len())
	}
	// Three items fit inside one sample, so eviction is exact LRU here.
	if n := c.EvictLRU(2); n != 1 || c.Len() != 2 {
		t.Fatalf("EvictLRU(2) evicted %d, len %d; want 1 and 2", n, c.Len())
	}
	if _, ok := c.items["b"]; ok {
		t.Fatal("EvictLRU kept the least-recently-used item")
	}
	// A touch refreshes an item's position.
	c.Touch("a", 40)
	if n := c.EvictLRU(1); n != 1 || c.Len() != 1 {
		t.Fatalf("EvictLRU(1) evicted %d, len %d; want 1 and 1", n, c.Len())
	}
	if _, ok := c.items["a"]; !ok {
		t.Fatal("EvictLRU evicted the most-recently-touched item")
	}
}
