/**
 * Basic node-redis client example against a local RAMen server.
 * Requires RAMen listening on localhost:6379.
 */
import { createClient } from "redis";

async function main() {
  const client = createClient({ url: "redis://localhost:6379" });
  client.on("error", (err) => console.error("Redis Client Error", err));
  await client.connect();

  await client.set("hello", "world");
  console.log("GET hello ->", await client.get("hello"));

  await client.hSet("user:1", { name: "Ada", role: "engineer" });
  console.log("HGETALL user:1 ->", await client.hGetAll("user:1"));

  await client.del("queue:jobs");
  await client.rPush("queue:jobs", ["job-a", "job-b", "job-c"]);
  console.log("LRANGE queue:jobs 0 -1 ->", await client.lRange("queue:jobs", 0, -1));

  try {
    await client.sendCommand(["VSET", "docs", "d1", "0.1", "0.2", "0.9", "META", "intro page"]);
    await client.sendCommand(["VSET", "docs", "d2", "0.9", "0.1", "0.0", "META", "pricing page"]);
    const result = await client.sendCommand([
      "VSEARCH", "docs", "0.1", "0.2", "0.85", "TOPK", "1", "WITHSCORES",
    ]);
    console.log("VSEARCH docs ... ->", result);
  } catch (err) {
    console.log("Vector commands unavailable or error:", err.message);
  }

  console.log("Done.");
  await client.quit();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
