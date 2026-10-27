# RAMen Command Reference

This documents every command implemented today. Syntax follows Redis
conventions; arguments in `<>` are required, `[]` optional. RAMen is single
-database (`SELECT 0` only) in this release.

## Connection & server

| Command | Description |
|---|---|
| `PING [msg]` | Returns `PONG`, or echoes `msg`. |
| `ECHO <msg>` | Returns `msg`. |
| `AUTH <password>` | Authenticate when `--auth` is set. |
| `SELECT <index>` | Only index `0` is accepted. |
| `QUIT` | Close the connection. |
| `COMMAND [COUNT]` | Minimal stub for client handshakes. |
| `INFO` | Server, client, keyspace, and cache stats. |
| `DBSIZE` | Number of live keys. |
| `FLUSHDB` / `FLUSHALL` | Remove all keys. |
| `SAVE` / `BGSAVE` | Write a snapshot to disk now. |

## Keys

| Command | Description |
|---|---|
| `DEL <key> [key ...]` | Delete keys; returns count removed. |
| `EXISTS <key> [key ...]` | Count of keys that exist. |
| `EXPIRE <key> <seconds>` | Set a TTL in seconds. |
| `PEXPIRE <key> <ms>` | Set a TTL in milliseconds. |
| `EXPIREAT <key> <unix-seconds>` | Expire at an absolute Unix time (seconds); a past time deletes the key. |
| `PEXPIREAT <key> <unix-ms>` | Expire at an absolute Unix time (milliseconds). |
| `TTL <key>` | Seconds left (`-1` no TTL, `-2` missing). |
| `PTTL <key>` | Milliseconds left. |
| `EXPIRETIME <key>` | Absolute Unix time in seconds the key expires at (`-1` no TTL, `-2` missing). |
| `PERSIST <key>` | Remove the TTL. |
| `KEYS <pattern>` | Glob-match keys (`*`, `?`, `[...]`). |
| `TYPE <key>` | `string`/`hash`/`list`/`set`/`zset`/`vector`/`none`. |
| `RENAME <key> <newkey>` | Rename a key, overwriting `newkey`; the TTL moves with it. Errors if the key is missing. |
| `RENAMENX <key> <newkey>` | Rename only if `newkey` does not exist; `1` if renamed, `0` if it already exists. |
| `RANDOMKEY` | Return a random key, or nil when the keyspace is empty. |

## Strings

| Command | Description |
|---|---|
| `SET <key> <value> [EX s\|PX ms] [NX\|XX]` | Set a string with options. |
| `SETNX <key> <value>` | Set only if the key does not exist; returns `1` or `0`. |
| `SETEX <key> <seconds> <value>` | Set with a TTL in seconds; rejects a non-positive TTL. |
| `PSETEX <key> <ms> <value>` | Set with a TTL in milliseconds; rejects a non-positive TTL. |
| `GET <key>` | Get a string. |
| `GETDEL <key>` | Get the value and delete the key atomically; nil if missing. |
| `GETEX <key> [EX s \| PX ms \| EXAT ts \| PXAT ts \| PERSIST]` | Get the value and optionally set or clear its TTL; no option leaves the TTL as-is. |
| `GETSET <key> <value>` | Set and return the old value. |
| `APPEND <key> <value>` | Append; returns new length. |
| `STRLEN <key>` | Length of the string in bytes; `0` if missing. |
| `GETRANGE <key> <start> <end>` | Substring by inclusive offsets; negative counts from the end. |
| `SETRANGE <key> <offset> <value>` | Overwrite from `offset`, zero-padding past the end; returns new length. |
| `INCR <key>` / `DECR <key>` | ±1 on an integer string. |
| `INCRBY <key> <n>` / `DECRBY <key> <n>` | ±n. |
| `INCRBYFLOAT <key> <n>` | Add float `n` to a float string; returns the new value. |
| `MGET <key> [key ...]` | Multiple gets. |
| `MSET <key> <val> [key val ...]` | Multiple sets. |
| `MSETNX <key> <val> [key val ...]` | Set all pairs only if none of the keys exist; `1` if set, `0` otherwise. |
| `SETBIT <key> <offset> <0\|1>` | Set or clear one bit (bit 0 is the high bit of the first byte), zero-padding past the end; returns the old bit. |
| `GETBIT <key> <offset>` | Read one bit; `0` past the end or for a missing key. |
| `BITCOUNT <key> [start end [BYTE\|BIT]]` | Count set bits, optionally in a byte (default) or bit range; negative indexes count from the end. |
| `BITOP <AND\|OR\|XOR\|NOT> <dest> <key> [key ...]` | Combine strings bit by bit into `dest`, zero-padding shorter ones; returns the result length. `NOT` takes one key. |

## Hashes

`HSET`, `HSETNX`, `HINCRBY`, `HINCRBYFLOAT`, `HGET`, `HDEL`, `HGETALL`, `HKEYS`, `HVALS`, `HLEN`, `HEXISTS`, `HMGET`.

```
HSET user:1 name Rohit plan pro
HSETNX user:1 visits 0  # set only if visits is missing
HINCRBY user:1 visits 1 # increment an integer field
HINCRBYFLOAT user:1 score 2.5
HGET user:1 name        # "Rohit"
HGETALL user:1
```

## Lists

`LPUSH`, `RPUSH`, `LPOP`, `RPOP`, `LLEN`, `LINDEX`, `LRANGE`, `LSET`, `LREM`, `LTRIM`, `LINSERT`.

```
RPUSH q a b c
LRANGE q 0 -1          # a b c
LPOP q                 # a
LSET q 0 A             # overwrite index 0 (errors if the key or index is missing)
LREM q 1 A             # remove up to 1 "A" from the head; -1 would start at the tail, 0 removes all
LTRIM q 0 1            # keep only indexes 0..1, drop the rest
LINSERT q BEFORE b Z   # insert Z before the first "b"; returns new length, or -1 if b is missing
```

## Sets

| Command | Description |
|---|---|
| `SADD <key> <member ...>` | Add members; returns the number newly added. |
| `SREM <key> <member ...>` | Remove members; returns the number removed. |
| `SMEMBERS <key>` | All members. |
| `SISMEMBER <key> <member>` | `1` if the member is in the set, else `0`. |
| `SCARD <key>` | Number of members. |
| `SINTER <key> [key ...]` | Members present in every set; a missing key is an empty set. |
| `SUNION <key> [key ...]` | Distinct members across all the sets. |
| `SDIFF <key> [key ...]` | Members of the first set that are in none of the rest. |
| `SPOP <key> [count]` | Remove and return one random member, or an array of up to `count`. |
| `SRANDMEMBER <key> [count]` | Return random member(s) without removing; a negative `count` allows repeats, capped at 2^20 per call. |

## Sorted sets

| Command | Description |
|---|---|
| `ZADD <key> <score> <member> [score member ...]` | Add or update members; returns the number newly added. Scores may be `inf`/`-inf`; a `nan` score is an error and nothing is written. |
| `ZREM <key> <member ...>` | Remove members; returns the number removed. |
| `ZSCORE <key> <member>` | The member's score, or nil. |
| `ZINCRBY <key> <increment> <member>` | Add `increment` to the member's score (creating it); returns the new score. |
| `ZCARD <key>` | Number of members. |
| `ZRANGE <key> <start> <stop> [WITHSCORES]` | Members in the rank range, lowest score first. |
| `ZRANGEBYSCORE <key> <min> <max> [WITHSCORES]` | Members with a score in `[min, max]`. |
| `ZRANK <key> <member>` | 0-based rank (lowest score first), or nil if missing. |
| `ZREVRANK <key> <member>` | 0-based rank (highest score first), or nil if missing. |
| `ZREVRANGE <key> <start> <stop> [WITHSCORES]` | Members in the rank range, highest score first. |
| `ZCOUNT <key> <min> <max>` | Count members with a score in `[min, max]`; prefix a bound with `(` for exclusive, and `-inf`/`+inf` are allowed. |

```
ZADD board 100 alice 250 bob
ZRANGE board 0 -1 WITHSCORES        # alice 100 bob 250
ZRANGEBYSCORE board 150 +inf        # bob
```

Scores are printed exactly as Redis prints them: whole numbers as plain digits
(`1000000`, not `1e+06`), small and large values in Redis's own exponent form
(`1e-7`, `1e+19`), and `inf`/`-inf` for infinities.

## Pub/Sub

`SUBSCRIBE <channel ...>`, `UNSUBSCRIBE [channel ...]`, `PUBLISH <channel> <message>`.

A slow subscriber that fills its buffer drops messages rather than blocking
publishers.

## Vector store

| Command | Description |
|---|---|
| `VSET <coll> <id> <f1 ... fn> [META <text>]` | Insert/replace a vector with optional metadata. |
| `VSEARCH <coll> <f1 ... fn> [TOPK k] [WITHSCORES]` | Cosine top-k search. Returns `id, meta` per hit (plus score with `WITHSCORES`). |
| `VDEL <coll> <id>` | Remove a vector. |
| `VCARD <coll>` | Number of vectors. |
| `VDIM <coll>` | Vector dimension of the collection. |

All vectors in a collection share the dimension fixed by the first `VSET`.

```
VSET docs d1 0.1 0.2 0.9 META "intro"
VSET docs d2 0.9 0.1 0.0 META "pricing"
VSEARCH docs 0.1 0.2 0.85 TOPK 1 WITHSCORES
```

## Semantic cache

Requires an embeddings provider (`RAMEN_EMBED_URL`, see the README). Stores and
retrieves responses keyed by prompt *meaning*.

| Command | Description |
|---|---|
| `SCACHE.SET <prompt> <response> [TTL <seconds>]` | Cache a response for a prompt. |
| `SCACHE.GET <prompt> [THRESHOLD <0..1>]` | Return the response if a stored prompt is similar enough (default threshold `0.9`); otherwise nil. |

Hits and misses are counted and surfaced via `INFO` and the dashboard.

An entry past its TTL stops matching immediately, and the background expiry
sweeper reclaims its memory even if it is never queried again. Entries without
a TTL stay until replaced or deleted.

### Dollar-savings demo

```
# Suppose each LLM completion costs $0.01.
SCACHE.SET "summarize the BSD-3 license" "<summary text>" TTL 3600
SCACHE.GET "give me a summary of the BSD 3-clause license"   # HIT -> $0.01 saved
```

Track the running hit ratio on the dashboard (`http://localhost:8080`) — every
hit is a model call you didn't pay for.

## Agent memory

| Command | Description |
|---|---|
| `REMEMBER <session> <field> <value>` | Store a fact under a session namespace. |
| `RECALL <session> [field]` | Recall one field, or the whole session. |

Backed by a per-session hash named `mem:<session>`; also exposed as MCP tools.
