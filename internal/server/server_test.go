package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/client"
	"github.com/Rohit-Dnath/RAMen/internal/store"
)

// startTestServer boots a server on an ephemeral port and returns a connected
// client plus a cleanup function.
func startTestServer(t *testing.T) (*client.Client, func()) {
	t.Helper()
	// Grab a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := New(store.New(), Config{Addr: addr})
	ctx, cancel := context.WithCancel(context.Background())
	go srv.ListenAndServe(ctx)

	// Wait for the listener to come up.
	var cli *client.Client
	for i := 0; i < 50; i++ {
		if cli, err = client.Dial(addr); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	return cli, func() { cli.Close(); cancel() }
}

func mustDo(t *testing.T, cli *client.Client, args ...string) any {
	t.Helper()
	r, err := cli.Do(args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	if e, ok := r.(error); ok {
		t.Fatalf("%v -> server error: %v", args, e)
	}
	return r
}

func TestCoreCommands(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "PING"); r != "PONG" {
		t.Fatalf("PING = %v", r)
	}
	mustDo(t, cli, "SET", "foo", "bar")
	if r := mustDo(t, cli, "GET", "foo"); r != "bar" {
		t.Fatalf("GET = %v", r)
	}
	if r := mustDo(t, cli, "EXISTS", "foo"); r != int64(1) {
		t.Fatalf("EXISTS = %v", r)
	}
	if r := mustDo(t, cli, "INCR", "n"); r != int64(1) {
		t.Fatalf("INCR = %v", r)
	}
	if r := mustDo(t, cli, "DEL", "foo"); r != int64(1) {
		t.Fatalf("DEL = %v", r)
	}
	if r, _ := cli.Do("GET", "foo"); r != nil {
		t.Fatalf("GET after DEL = %v", r)
	}
}

func TestDataStructures(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "RPUSH", "l", "a", "b", "c")
	if r := mustDo(t, cli, "LLEN", "l"); r != int64(3) {
		t.Fatalf("LLEN = %v", r)
	}
	r := mustDo(t, cli, "LRANGE", "l", "0", "-1").([]any)
	if len(r) != 3 || r[0] != "a" || r[2] != "c" {
		t.Fatalf("LRANGE = %v", r)
	}

	mustDo(t, cli, "HSET", "h", "f1", "v1", "f2", "v2")
	if r := mustDo(t, cli, "HGET", "h", "f1"); r != "v1" {
		t.Fatalf("HGET = %v", r)
	}

	mustDo(t, cli, "SADD", "s", "x", "y", "x")
	if r := mustDo(t, cli, "SCARD", "s"); r != int64(2) {
		t.Fatalf("SCARD = %v", r)
	}

	mustDo(t, cli, "ZADD", "z", "1", "one", "2", "two")
	zr := mustDo(t, cli, "ZRANGE", "z", "0", "-1").([]any)
	if len(zr) != 2 || zr[0] != "one" || zr[1] != "two" {
		t.Fatalf("ZRANGE = %v", zr)
	}
}

func TestVectorCommands(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "VSET", "vc", "a", "1", "0", "0", "META", "alpha")
	mustDo(t, cli, "VSET", "vc", "b", "0", "1", "0", "META", "beta")
	if r := mustDo(t, cli, "VCARD", "vc"); r != int64(2) {
		t.Fatalf("VCARD = %v", r)
	}
	res := mustDo(t, cli, "VSEARCH", "vc", "1", "0", "0", "TOPK", "1").([]any)
	if len(res) != 2 || res[0] != "a" || res[1] != "alpha" {
		t.Fatalf("VSEARCH = %v", res)
	}
}

func TestWrongTypeOverWire(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()
	mustDo(t, cli, "RPUSH", "l", "a")
	r, err := cli.Do("GET", "l")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.(error); !ok {
		t.Fatalf("expected WRONGTYPE error, got %v", r)
	}
}

func TestAgentMemory(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()
	mustDo(t, cli, "REMEMBER", "sess1", "name", "Rohit")
	if r := mustDo(t, cli, "RECALL", "sess1", "name"); r != "Rohit" {
		t.Fatalf("RECALL = %v", r)
	}
}

func TestPubSub(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()
	// SUBSCRIBE should acknowledge with ["subscribe", channel, count].
	r := mustDo(t, cli, "SUBSCRIBE", "ch").([]any)
	if r[0] != "subscribe" || r[1] != "ch" || r[2] != int64(1) {
		t.Fatalf("SUBSCRIBE ack = %v", r)
	}
	// A publisher on a second connection should see one subscriber.
	pub, cleanup2 := startTestServerClientOn(t, cli)
	defer cleanup2()
	if n := mustDo(t, pub, "PUBLISH", "ch", "hello"); n != int64(1) {
		t.Fatalf("PUBLISH delivered to %v subscribers, want 1", n)
	}
}

// startTestServerClientOn opens a second client to the same server the given
// client is connected to.
func startTestServerClientOn(t *testing.T, cli *client.Client) (*client.Client, func()) {
	t.Helper()
	c2, err := client.Dial(cli.RemoteAddr())
	if err != nil {
		t.Fatalf("dial second client: %v", err)
	}
	return c2, func() { c2.Close() }
}
