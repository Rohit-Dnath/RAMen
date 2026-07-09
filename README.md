<p align="center">
  <img src="https://res.cloudinary.com/dfibwwpbl/image/upload/v1781639895/ChatGPT_Image_Jun_17_2026_01_27_59_AM_kmcncj.png" alt="RAMen" width="100%">
</p>

<h1 align="center">RAMen</h1>

<p align="center"><b>RAM + ramen. In memory, and fast to spin up.</b></p>

<p align="center">
  <a href="https://github.com/Rohit-Dnath/RAMen/actions/workflows/ci.yml"><img src="https://github.com/Rohit-Dnath/RAMen/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-V1%20%2B%20V2%20preview-orange" alt="status">
  <img src="https://img.shields.io/badge/go-1.25-00ADD8" alt="go">
  <img src="https://img.shields.io/badge/license-BSD--3--Clause-blue" alt="license">
  <a href="https://wise.com/pay/business/rohitdebnath2?utm_source=open_link"><img src="https://img.shields.io/badge/support-RAMen-9FE870?logo=wise&logoColor=163300&labelColor=163300" alt="Support RAMen"></a>
</p>

<p align="center">
  <a href="https://github.com/Rohit-Dnath/RAMen/stargazers"><img src="https://img.shields.io/github/stars/Rohit-Dnath/RAMen?style=social" alt="stars"></a>
  <a href="https://github.com/Rohit-Dnath/RAMen/labels/good%20first%20issue"><img src="https://img.shields.io/github/issues/Rohit-Dnath/RAMen/good%20first%20issue?color=7057ff&label=good%20first%20issues" alt="good first issues"></a>
  <a href="https://github.com/Rohit-Dnath/RAMen/issues"><img src="https://img.shields.io/github/issues/Rohit-Dnath/RAMen?color=008672" alt="open issues"></a>
  <a href="https://github.com/Rohit-Dnath/RAMen/graphs/contributors"><img src="https://img.shields.io/github/contributors/Rohit-Dnath/RAMen?color=orange" alt="contributors"></a>
  <a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen" alt="PRs welcome"></a>
</p>

RAMen is a fast in-memory data store, like Redis, but built for AI apps and AI agents.

It does three things:

1. It works as a drop-in cache. It speaks the same protocol as Redis, so your existing Redis client and code keep working with no changes.
2. It can store and search vectors, and it can cache AI answers by meaning. So if two prompts mean the same thing, RAMen can return the saved answer instead of calling the model again. That saves you money.
3. It has a built-in MCP server. That means AI agents (like Claude) can read, write, search, and remember data in RAMen directly as a tool. No extra glue code needed.

> Redis was built for app servers. RAMen is built for AI agents.

## Is this a Redis alternative?

Yes, for a lot of common use cases. If you use Redis (or Valkey) as a cache or a simple key value store, you can point your app at RAMen instead and it will just work, because RAMen speaks the Redis protocol (RESP2).

Where RAMen is different:

- It has vector search and a semantic cache built in. With Redis you would need an extra module or extra code for that.
- It has an MCP server built in, so AI agents can use it as a tool out of the box.
- It is one small single file (a single binary) with no extra dependencies. Easy to download and run.

What RAMen does NOT do yet (so you know what you are getting):

- No clustering, no replication, no failover.
- No append-only-file durability (it saves snapshots to disk instead).
- Not tuned to beat Redis or Valkey on raw speed.

So: great as a Redis-style cache and an AI data layer for one machine. Not yet a replacement for a big production Redis cluster.

## Quick start (the simple version)

You need [Go](https://go.dev/dl/) installed (version 1.25 or newer). Then run one of these.

### Option 1: install with Go

```bash
go install github.com/Rohit-Dnath/RAMen/cmd/ramen@latest
ramen
```

### Option 2: run with Docker

```bash
docker run -p 6379:6379 -p 8080:8080 ghcr.io/rohit-dnath/ramen:latest
```

### Option 3: build from source

```bash
git clone https://github.com/Rohit-Dnath/RAMen
cd RAMen
go run ./cmd/ramen
```

When it starts you will see:

```
ramen: listening on [::]:6379
ramen: dashboard on http://localhost:8080
```

That is it. RAMen is now running on port 6379, and a live dashboard is at http://localhost:8080.

## Client examples

- [Go (go-redis)](examples/go/README.md) — SET/GET, hash, list, and VSET/VSEARCH

## How to use it

### Talk to it like Redis

If you have `redis-cli` installed, just connect to port 6379:

```bash
redis-cli -p 6379
```

```
> SET hello world
OK
> GET hello
"world"
> EXPIRE hello 60
(integer) 1
```

If you have an app that already uses Redis, point it at RAMen. Usually you only change one setting:

```bash
export REDIS_URL=redis://localhost:6379
```

### Basic commands

These work just like Redis:

```
SET user:1 "Rohit"          store a value
GET user:1                  read it back
INCR visits                 count something
HSET person name Rohit      store fields under one key
RPUSH queue job1 job2       a list
SADD tags go ai             a set
ZADD board 100 alice        a ranked list
SUBSCRIBE news              listen for messages
PUBLISH news "hello"        send a message
```

Full list with examples: [docs/commands.md](docs/commands.md).

### Vector search

Store vectors and find the closest ones. Your app gives RAMen the numbers, no API key needed.

```
VSET docs d1 0.1 0.2 0.9 META "intro page"
VSET docs d2 0.9 0.1 0.0 META "pricing page"
VSEARCH docs 0.1 0.2 0.85 TOPK 1 WITHSCORES
```

### Semantic cache (save money on AI calls)

Cache an AI answer once. Next time a similar question comes in, get the saved answer back instead of paying for another model call.

```
SCACHE.SET "What is the capital of France?" "Paris"
SCACHE.GET "whats the capital of france" THRESHOLD 0.9
> "Paris"
```

This needs an embeddings provider. It works with a free local [Ollama](https://ollama.com) or with OpenAI. See [Configuration](#configuration).

### Agent memory

Let an AI agent remember things across turns:

```
REMEMBER session7 user_name Rohit
RECALL session7 user_name
> "Rohit"
```

### Use it from Claude (MCP)

Start RAMen, then add this to your Claude config and Claude can use RAMen as a tool:

```json
{
  "mcpServers": {
    "ramen": {
      "command": "ramen",
      "args": ["mcp", "--addr", "localhost:6379"]
    }
  }
}
```

Step by step guide: [docs/mcp.md](docs/mcp.md).

### The dashboard

Open http://localhost:8080 in your browser while RAMen is running. You will see your keys, memory use, and the cache hit rate, updating live. Turn it off with `--dashboard-addr ""`.

## RAMen vs Redis vs Valkey

| | RAMen | Redis | Valkey |
|---|---|---|---|
| Redis protocol (RESP2) | Yes | Yes | Yes |
| Works with existing Redis clients | Yes (common commands) | Yes | Yes |
| One single binary, no setup | Yes | No | No |
| Vector search built in | Yes | Needs a module | Needs a module |
| Semantic cache command | Yes | No | No |
| MCP server for AI agents | Yes | No | No |
| Clustering and replication | Not yet | Yes | Yes |
| Best raw speed on one node | Good | Best | Best |
| License | BSD-3-Clause | AGPL / RSAL (since 2024) | BSD-3-Clause |

## Configuration

Every flag also has a `RAMEN_*` environment variable.

| Flag | Env var | Default | What it does |
|---|---|---|---|
| `--addr` | `RAMEN_ADDR` | `:6379` | Port to listen on |
| `--auth` | `RAMEN_AUTH` | empty | Require a password (empty means no password) |
| `--snapshot-path` | `RAMEN_SNAPSHOT_PATH` | `ramen.snapshot` | Where to save data (empty turns saving off) |
| `--snapshot-interval` | `RAMEN_SNAPSHOT_INTERVAL` | `60s` | How often to save to disk |
| `--dashboard-addr` | `RAMEN_DASHBOARD_ADDR` | `:8080` | Dashboard port (empty turns it off) |
| `RAMEN_EMBED_URL` | `RAMEN_EMBED_URL` | empty | Embeddings endpoint for the semantic cache |
| `RAMEN_EMBED_MODEL` | `RAMEN_EMBED_MODEL` | `text-embedding-3-small` | Which embedding model to use |
| `RAMEN_EMBED_KEY` | `RAMEN_EMBED_KEY` | empty | API key for the embeddings endpoint |

Turn on the semantic cache for free with a local Ollama:

```bash
ollama pull nomic-embed-text
export RAMEN_EMBED_URL=http://localhost:11434/v1/embeddings
export RAMEN_EMBED_MODEL=nomic-embed-text
ramen
```

Or use OpenAI:

```bash
export RAMEN_EMBED_URL=https://api.openai.com/v1/embeddings
export RAMEN_EMBED_MODEL=text-embedding-3-small
export RAMEN_EMBED_KEY=sk-your-key
ramen
```

## Roadmap

- Done: core Redis commands, strings, hashes, lists, sets, sorted sets, expiry, pub/sub, snapshot saving.
- Done: vector store, semantic cache, agent memory, MCP server, dashboard.
- Next: replication, stronger durability, clustering, faster vector index (HNSW).

## Support the project

RAMen is free and open source, and it always will be. If it saves you time or you would like to help fund its development, you can support the work directly. It genuinely helps keep the project moving.

<p align="center">
  <a href="https://wise.com/pay/business/rohitdebnath2?utm_source=open_link">
    <img src="https://img.shields.io/badge/Support%20RAMen%20via%20Wise-9FE870?style=for-the-badge&logo=wise&logoColor=163300&labelColor=9FE870" alt="Support RAMen via Wise">
  </a>
</p>

Not in a position to chip in? A star, a share, or a bug report helps just as much. Thank you.

## Contributing

RAMen is open source and very new, which means this is a great time to get involved and make a real difference. Beginners are welcome. You do not need to be a Go expert.

Good first things to help with:

- Try it out and report bugs or confusing parts.
- Improve the docs or add examples.
- Add a missing Redis command.
- Test RAMen with your favourite Redis client and tell us what worked.

### Good first issues

New here? These are small, self-contained, and well described. Pick one, comment to claim it, and you are off.

- [**Good first issues**](https://github.com/Rohit-Dnath/RAMen/labels/good%20first%20issue) beginner-friendly, clearly scoped tasks.
- [**Help wanted**](https://github.com/Rohit-Dnath/RAMen/labels/help%20wanted) things we would love a hand with.
- [**All open issues**](https://github.com/Rohit-Dnath/RAMen/issues) the full list.

Most open issues are missing Redis commands with the exact files and acceptance criteria already written down, so you can follow an existing handler as a template and open a PR the same day.

Then read the [Contributing Guide](CONTRIBUTING.md) to get started. If you are not sure where to begin, open an issue and say hi. Stars and shares also help a lot.

## Contributors

Thanks to these wonderful people who help make RAMen better.

<table>
  <tr>
    <td align="center">
      <a href="https://github.com/Rohit-Dnath">
        <img src="https://avatars.githubusercontent.com/u/136178612?v=4" width="90" height="90" style="border-radius:50%" alt="Rohit-Dnath"><br>
        <sub><b>Rohit-Dnath</b></sub><br>
        <img src="https://img.shields.io/badge/%F0%9F%8D%9C%20Creator%20%26%20Maintainer-E8590C?style=for-the-badge&labelColor=E8590C" alt="Creator & Maintainer">
      </a>
    </td>
    <td align="center">
      <a href="https://github.com/bayrambartu">
        <img src="https://avatars.githubusercontent.com/u/113308882?v=4" width="80" height="80" style="border-radius:50%" alt="bayrambartu"><br>
        <sub><b>bayrambartu</b></sub><br>
        <img src="https://img.shields.io/badge/Contributor-495057?style=for-the-badge&labelColor=495057" alt="Contributor">
      </a>
    </td>
    <td align="center">
      <a href="https://github.com/tchalikanti1705">
        <img src="https://avatars.githubusercontent.com/u/198486914?v=4" width="80" height="80" style="border-radius:50%" alt="tchalikanti1705"><br>
        <sub><b>tchalikanti1705</b></sub><br>
        <img src="https://img.shields.io/badge/Contributor-495057?style=for-the-badge&labelColor=495057" alt="Contributor">
      </a>
    </td>
  </tr>
</table>

## Star History

<p align="center">
  <a href="https://www.star-history.com/#Rohit-Dnath/RAMen&Date">
    <img src="assets/star-history.png" alt="RAMen Star History Chart" width="80%">
  </a>
</p>

## License

[BSD-3-Clause](LICENSE). This is the same license family Redis used before 2024, and the one Valkey uses. Simple and permissive, no surprises.
