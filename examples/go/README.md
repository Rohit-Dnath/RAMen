# RAMen Go client example

Uses [go-redis](https://github.com/redis/go-redis) against a local RAMen server.

## Prerequisites

Start RAMen on port 6379, then:

```bash
cd examples/go
go run .
```

## What it demonstrates

- SET / GET
- Hash (HSet / HGetAll)
- List (RPush / LRange)
- RAMen vector commands (VSET / VSEARCH) via `Do`
