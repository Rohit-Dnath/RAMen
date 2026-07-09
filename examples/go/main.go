package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	if err := rdb.Set(ctx, "hello", "world", 0).Err(); err != nil {
		log.Fatal(err)
	}
	val, err := rdb.Get(ctx, "hello").Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("GET hello ->", val)

	if err := rdb.HSet(ctx, "user:1", "name", "Ada", "role", "engineer").Err(); err != nil {
		log.Fatal(err)
	}
	user, err := rdb.HGetAll(ctx, "user:1").Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("HGETALL user:1 ->", user)

	_ = rdb.Del(ctx, "queue:jobs").Err()
	if err := rdb.RPush(ctx, "queue:jobs", "job-a", "job-b", "job-c").Err(); err != nil {
		log.Fatal(err)
	}
	jobs, err := rdb.LRange(ctx, "queue:jobs", 0, -1).Result()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("LRANGE queue:jobs 0 -1 ->", jobs)

	// RAMen-specific vector commands
	if err := rdb.Do(ctx, "VSET", "docs", "d1", 0.1, 0.2, 0.9, "META", "intro page").Err(); err != nil {
		fmt.Println("Vector commands unavailable or error:", err)
	} else {
		_ = rdb.Do(ctx, "VSET", "docs", "d2", 0.9, 0.1, 0.0, "META", "pricing page").Err()
		res, err := rdb.Do(ctx, "VSEARCH", "docs", 0.1, 0.2, 0.85, "TOPK", 1, "WITHSCORES").Result()
		if err != nil {
			fmt.Println("VSEARCH error:", err)
		} else {
			fmt.Println("VSEARCH docs ... ->", res)
		}
	}

	fmt.Println("Done.")
}
