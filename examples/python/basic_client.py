"""Basic redis-py client example against a local RAMen server.

Requires RAMen listening on localhost:6379 (default).
"""

from __future__ import annotations

import redis


def main() -> None:
    client = redis.Redis(host="localhost", port=6379, decode_responses=True)

    # --- Strings ---
    client.set("hello", "world")
    print("GET hello ->", client.get("hello"))

    # --- Hash ---
    client.hset("user:1", mapping={"name": "Ada", "role": "engineer"})
    print("HGETALL user:1 ->", client.hgetall("user:1"))

    # --- List ---
    client.delete("queue:jobs")
    client.rpush("queue:jobs", "job-a", "job-b", "job-c")
    print("LRANGE queue:jobs 0 -1 ->", client.lrange("queue:jobs", 0, -1))

    # --- RAMen-specific: vector set + search ---
    # VSET <index> <id> <f1> <f2> ... [META <text>]
    # VSEARCH <index> <q1> <q2> ... TOPK <k> [WITHSCORES]
    try:
        client.execute_command("VSET", "docs", "d1", 0.1, 0.2, 0.9, "META", "intro page")
        client.execute_command("VSET", "docs", "d2", 0.9, 0.1, 0.0, "META", "pricing page")
        result = client.execute_command(
            "VSEARCH", "docs", 0.1, 0.2, 0.85, "TOPK", 1, "WITHSCORES"
        )
        print("VSEARCH docs ... ->", result)
    except redis.ResponseError as exc:
        print("Vector commands unavailable or error:", exc)
        print("Core Redis-compatible commands above still work.")

    print("Done.")


if __name__ == "__main__":
    main()
