# Benchmarking RAMen

This guide shows how to benchmark a local RAMen instance using `redis-benchmark`.

## Prerequisites

Start RAMen using Docker Compose:

```bash
docker compose up
```

This exposes:

- RESP server: `localhost:6379`
- Dashboard: `http://localhost:8080`

## Running benchmarks

### Linux/macOS

```bash
redis-benchmark -h localhost -p 6379 \
  -t ping,set,get,incr,lpush,rpush,lpop,sadd,hset,zadd
```

### Windows (PowerShell)

```powershell
docker run --rm redis redis-benchmark -h host.docker.internal -p 6379 -t ping,set,get,incr,lpush,rpush,lpop,sadd,hset,zadd
```

## Notes

- `redis-benchmark` may print:

```
WARNING: Could not fetch server CONFIG
```

This warning is expected because redis-benchmark tries to call Redis's CONFIG command to gather server information before running benchmarks. If RAMen doesn't implement CONFIG, it prints a warning but continues benchmarking normally. It does not prevent the benchmarks from running.

- The benchmark categories listed above were verified against the Docker Compose setup.
- No unsupported benchmark categories were identified during testing.

## VSEARCH scaling

Vector search is a flat scan: every query reads every vector in the collection.
Norms are cached at insert time and top-k selection uses a bounded min-heap, so
per-query allocation stays constant, but query time grows linearly with the
collection. There is no ANN index today (#13 tracks that decision), so this
table is the ceiling to check before putting a large collection on RAMen.

`BenchmarkVSearch`, top-10, 768 dimensions, seeded vectors:

| vectors | time/query | B/op |
|--------:|-----------:|-----:|
| 1k      | ~0.6 ms    | 664  |
| 10k     | ~6 ms      | 664  |
| 100k    | ~70 ms     | 664  |

Times vary with hardware (a laptop-class i7 measured ~114 ms at 100k); the
linear shape and the flat allocations do not. The scan is memory-bound — it
reads all of every vector per query — so faster arithmetic does not lower the
ceiling, only a smaller collection or an index does.

For the semantic cache this is not a bottleneck: a cache lookup in front of an
LLM call costing hundreds of milliseconds spends a few percent of what a hit
saves. Standalone `VSEARCH` over 100k+ vectors is where the ceiling matters.

Reproduce with:

```bash
go test -bench BenchmarkVSearch -benchmem ./internal/vector/
```
