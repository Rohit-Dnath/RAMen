# RAMen Python client example

Minimal script using stock [redis-py](https://github.com/redis/redis-py) against a local RAMen server.

## Prerequisites

1. Start RAMen on port 6379:

```bash
# from repo root
go run ./cmd/ramen
# or
docker run -p 6379:6379 -p 8080:8080 ghcr.io/rohit-dnath/ramen:latest
```

2. Install deps:

```bash
cd examples/python
pip install -r requirements.txt
```

## Run

```bash
python basic_client.py
```

## What it demonstrates

- `SET` / `GET`
- Hash (`HSET` / `HGETALL`)
- List (`RPUSH` / `LRANGE`)
- RAMen vector commands via `execute_command` (`VSET` / `VSEARCH`)
