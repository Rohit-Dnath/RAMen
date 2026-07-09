# RAMen Node.js client example

Uses the official [`redis`](https://www.npmjs.com/package/redis) package against a local RAMen server.

## Prerequisites

Start RAMen on port 6379, then:

```bash
cd examples/nodejs
npm install
npm start
```

## What it demonstrates

- SET / GET
- Hash (HSET / HGETALL)
- List (RPUSH / LRANGE)
- RAMen vector commands (VSET / VSEARCH) via `sendCommand`
