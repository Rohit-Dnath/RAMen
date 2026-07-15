package server

import (
	"context"
	"math"
	"net"
	"strconv"
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

func mustError(t *testing.T, cli *client.Client, args ...string) error {
	t.Helper()
	r, err := cli.Do(args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	e, ok := r.(error)
	if !ok {
		t.Fatalf("%v: expected server error, got %v", args, r)
	}
	return e
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
	mustDo(t, cli, "SET", "src", "value")
	mustError(t, cli, "COPY", "src1", "dst")
	if r := mustDo(t, cli, "COPY", "src", "dst"); r != "1" {
		t.Fatalf("COPY = %v", r)
	}
	mustError(t, cli, "COPY", "src", "dst")
	if r := mustDo(t, cli, "COPY", "src", "dst", "REPLACE"); r != "1" {
		t.Fatalf("COPY with REPLACE = %v", r)
	}
}

func TestDataStructures(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "RPUSH", "nums", "1", "10", "2")
	if r := mustDo(t, cli, "LLEN", "nums"); r != int64(3) {
		t.Fatalf("LLEN = %v", r)
	}
	r := mustDo(t, cli, "LRANGE", "nums", "0", "-1").([]any)
	if len(r) != 3 || r[0] != "1" || r[1] != "10" || r[2] != "2" {
		t.Fatalf("LRANGE = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums").([]any)
	if len(r) != 3 || r[0] != "1" || r[1] != "2" || r[2] != "10" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "ALPHA").([]any)
	if len(r) != 3 || r[0] != "1" || r[1] != "10" || r[2] != "2" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "LIMIT", "1", "2").([]any)
	if len(r) != 2 || r[0] != "2" || r[1] != "10" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "LIMIT", "1", "100").([]any)
	if len(r) != 2 || r[0] != "2" || r[1] != "10" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "DESC").([]any)
	if len(r) != 3 || r[0] != "10" || r[1] != "2" || r[2] != "1" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "DESC", "LIMIT", "1", "2").([]any)
	if len(r) != 2 || r[0] != "2" || r[1] != "1" {
		t.Fatalf("SORT = %v", r)
	}
	r = mustDo(t, cli, "SORT", "nums", "LIMIT", "0", "2", "DESC").([]any)
	if len(r) != 2 || r[0] != "10" || r[1] != "2" {
		t.Fatalf("SORT = %v", r)
	}
	mustDo(t, cli, "HSET", "h", "f1", "v1", "f2", "v2")
	if r := mustDo(t, cli, "HGET", "h", "f1"); r != "v1" {
		t.Fatalf("HGET = %v", r)
	}

	if r := mustDo(t, cli, "HSETNX", "h", "f3", "v3"); r != int64(1) {
		t.Fatalf("HSETNX new = %v", r)
	}
	if r := mustDo(t, cli, "HSETNX", "h", "f3", "v4"); r != int64(0) {
		t.Fatalf("HSETNX existing = %v", r)
	}
	if r := mustDo(t, cli, "HGET", "h", "f3"); r != "v3" {
		t.Fatalf("HSETNX overwrote existing field: %v", r)
	}
	if r := mustDo(t, cli, "HSETNX", "h", "", ""); r != int64(1) {
		t.Fatalf("HSETNX empty field/value = %v", r)
	}
	if r := mustDo(t, cli, "HGET", "h", ""); r != "" {
		t.Fatalf("HGET empty field = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBY", "h", "count", "5"); r != int64(5) {
		t.Fatalf("HINCRBY create = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBY", "h", "count", "-2"); r != int64(3) {
		t.Fatalf("HINCRBY existing = %v", r)
	}
	if r := mustDo(t, cli, "HGET", "h", "count"); r != "3" {
		t.Fatalf("HGET after HINCRBY = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBYFLOAT", "h", "score", "1.5"); r != "1.5" {
		t.Fatalf("HINCRBYFLOAT create = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBYFLOAT", "h", "score", "2.25"); r != "3.75" {
		t.Fatalf("HINCRBYFLOAT existing = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBYFLOAT", "h", "score", "-3.75"); r != "0" {
		t.Fatalf("HINCRBYFLOAT zero normalization = %v", r)
	}
	if r := mustDo(t, cli, "HINCRBYFLOAT", "h", "whole", "5"); r != "5" {
		t.Fatalf("HINCRBYFLOAT integer-valued = %v", r)
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

func TestHashSetNXErrors(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustError(t, cli, "HSETNX", "h", "field")
	mustDo(t, cli, "SET", "str", "value")
	mustError(t, cli, "HSETNX", "str", "field", "value")
}

func TestHashIncrByErrors(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustError(t, cli, "HINCRBY", "h", "field")
	mustError(t, cli, "HINCRBY", "h", "field", "not-an-int")
	mustError(t, cli, "HINCRBY", "h", "field", "9223372036854775808")

	mustDo(t, cli, "HSET", "h", "bad", "abc")
	mustError(t, cli, "HINCRBY", "h", "bad", "1")
	if r := mustDo(t, cli, "HGET", "h", "bad"); r != "abc" {
		t.Fatalf("HINCRBY changed bad field = %v", r)
	}

	max := strconv.FormatInt(math.MaxInt64, 10)
	mustDo(t, cli, "HSET", "h", "max", max)
	mustError(t, cli, "HINCRBY", "h", "max", "1")
	if r := mustDo(t, cli, "HGET", "h", "max"); r != max {
		t.Fatalf("HINCRBY changed overflow field = %v", r)
	}

	min := strconv.FormatInt(math.MinInt64, 10)
	mustDo(t, cli, "HSET", "h", "min", min)
	mustError(t, cli, "HINCRBY", "h", "min", "-1")
	if r := mustDo(t, cli, "HGET", "h", "min"); r != min {
		t.Fatalf("HINCRBY changed underflow field = %v", r)
	}

	mustDo(t, cli, "SET", "str", "value")
	mustError(t, cli, "HINCRBY", "str", "field", "1")
}

func TestHashIncrByFloatErrors(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustError(t, cli, "HINCRBYFLOAT", "h", "field")
	mustError(t, cli, "HINCRBYFLOAT", "h", "field", "not-a-float")
	mustError(t, cli, "HINCRBYFLOAT", "h", "field", "NaN")
	mustError(t, cli, "HINCRBYFLOAT", "h", "field", "+Inf")
	mustError(t, cli, "HINCRBYFLOAT", "h", "field", "1e309")

	mustDo(t, cli, "HSET", "h", "bad", "abc")
	mustError(t, cli, "HINCRBYFLOAT", "h", "bad", "1")
	if r := mustDo(t, cli, "HGET", "h", "bad"); r != "abc" {
		t.Fatalf("HINCRBYFLOAT changed bad field = %v", r)
	}

	mustDo(t, cli, "HSET", "h", "huge", "1e308")
	mustError(t, cli, "HINCRBYFLOAT", "h", "huge", "1e308")
	if r := mustDo(t, cli, "HGET", "h", "huge"); r != "1e308" {
		t.Fatalf("HINCRBYFLOAT changed overflow field = %v", r)
	}

	mustDo(t, cli, "SET", "str", "value")
	mustError(t, cli, "HINCRBYFLOAT", "str", "field", "1.5")
}

func TestIncrByFloat(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "INCRBYFLOAT", "f", "1.5"); r != "1.5" {
		t.Fatalf("INCRBYFLOAT create = %v", r)
	}
	if r := mustDo(t, cli, "INCRBYFLOAT", "f", "2.25"); r != "3.75" {
		t.Fatalf("INCRBYFLOAT existing = %v", r)
	}
	if r := mustDo(t, cli, "INCRBYFLOAT", "f", "-3.75"); r != "0" {
		t.Fatalf("INCRBYFLOAT zero normalization = %v", r)
	}
	if r := mustDo(t, cli, "INCRBYFLOAT", "whole", "5"); r != "5" {
		t.Fatalf("INCRBYFLOAT integer-valued = %v", r)
	}
	mustDo(t, cli, "SET", "sci", "1e3")
	if r := mustDo(t, cli, "INCRBYFLOAT", "sci", "0.5"); r != "1000.5" {
		t.Fatalf("INCRBYFLOAT scientific value = %v", r)
	}
}

func TestIncrByFloatErrors(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustError(t, cli, "INCRBYFLOAT", "f")
	mustError(t, cli, "INCRBYFLOAT", "f", "1", "2")
	mustError(t, cli, "INCRBYFLOAT", "f", "not-a-float")
	mustError(t, cli, "INCRBYFLOAT", "f", "NaN")
	mustError(t, cli, "INCRBYFLOAT", "f", "+Inf")
	mustError(t, cli, "INCRBYFLOAT", "f", "1e309")

	mustDo(t, cli, "SET", "bad", "abc")
	mustError(t, cli, "INCRBYFLOAT", "bad", "1")
	if r := mustDo(t, cli, "GET", "bad"); r != "abc" {
		t.Fatalf("INCRBYFLOAT changed bad value = %v", r)
	}

	mustDo(t, cli, "SET", "infv", "inf")
	mustError(t, cli, "INCRBYFLOAT", "infv", "1")

	mustDo(t, cli, "SET", "huge", "1e308")
	mustError(t, cli, "INCRBYFLOAT", "huge", "1e308")
	if r := mustDo(t, cli, "GET", "huge"); r != "1e308" {
		t.Fatalf("INCRBYFLOAT changed overflow value = %v", r)
	}

	mustDo(t, cli, "RPUSH", "l", "a")
	mustError(t, cli, "INCRBYFLOAT", "l", "1.5")
}

func TestListSet(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustError(t, cli, "LSET", "l", "0", "x") // no such key
	mustDo(t, cli, "RPUSH", "l", "a", "b", "c")
	if r := mustDo(t, cli, "LSET", "l", "1", "B"); r != "OK" {
		t.Fatalf("LSET = %v", r)
	}
	if r := mustDo(t, cli, "LINDEX", "l", "1"); r != "B" {
		t.Fatalf("LSET did not update = %v", r)
	}
	if r := mustDo(t, cli, "LSET", "l", "-1", "C"); r != "OK" {
		t.Fatalf("LSET negative index = %v", r)
	}
	mustError(t, cli, "LSET", "l", "3", "z")        // index out of range
	mustError(t, cli, "LSET", "l", "notanint", "z") // bad index
	mustError(t, cli, "LSET", "l")                  // arity
	mustDo(t, cli, "SET", "str", "v")
	mustError(t, cli, "LSET", "str", "0", "x") // wrong type
}

func TestListRem(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "LREM", "nope", "0", "a"); r != int64(0) {
		t.Fatalf("LREM missing key = %v", r)
	}
	mustDo(t, cli, "RPUSH", "l", "a", "b", "a", "c", "a")
	if r := mustDo(t, cli, "LREM", "l", "2", "a"); r != int64(2) {
		t.Fatalf("LREM count>0 = %v", r)
	}
	rr := mustDo(t, cli, "LRANGE", "l", "0", "-1").([]any)
	if len(rr) != 3 || rr[0] != "b" || rr[1] != "c" || rr[2] != "a" {
		t.Fatalf("LREM result = %v", rr)
	}
	mustDo(t, cli, "RPUSH", "m", "a", "b", "a")
	if r := mustDo(t, cli, "LREM", "m", "-9223372036854775808", "a"); r != int64(2) {
		t.Fatalf("LREM int64-min count = %v", r) // -count overflows, must still remove all
	}
	mustError(t, cli, "LREM", "l", "notanint", "a") // bad count
	mustError(t, cli, "LREM", "l")                  // arity
	mustDo(t, cli, "SET", "str", "v")
	mustError(t, cli, "LREM", "str", "0", "x") // wrong type
}

func TestListTrim(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "LTRIM", "nope", "0", "-1"); r != "OK" {
		t.Fatalf("LTRIM missing key = %v", r)
	}
	mustDo(t, cli, "RPUSH", "l", "a", "b", "c", "d", "e")
	if r := mustDo(t, cli, "LTRIM", "l", "1", "3"); r != "OK" {
		t.Fatalf("LTRIM = %v", r)
	}
	rr := mustDo(t, cli, "LRANGE", "l", "0", "-1").([]any)
	if len(rr) != 3 || rr[0] != "b" || rr[2] != "d" {
		t.Fatalf("LTRIM result = %v", rr)
	}
	mustError(t, cli, "LTRIM", "l", "x", "3") // bad index
	mustError(t, cli, "LTRIM", "l", "1")      // arity
	mustDo(t, cli, "SET", "str", "v")
	mustError(t, cli, "LTRIM", "str", "0", "-1") // wrong type
}

func TestListInsert(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "LINSERT", "nope", "BEFORE", "a", "x"); r != int64(0) {
		t.Fatalf("LINSERT missing key = %v", r)
	}
	mustDo(t, cli, "RPUSH", "l", "a", "b", "c")
	if r := mustDo(t, cli, "LINSERT", "l", "BEFORE", "b", "X"); r != int64(4) {
		t.Fatalf("LINSERT before = %v", r)
	}
	if r := mustDo(t, cli, "LINSERT", "l", "after", "b", "Y"); r != int64(5) { // case-insensitive
		t.Fatalf("LINSERT after = %v", r)
	}
	rr := mustDo(t, cli, "LRANGE", "l", "0", "-1").([]any)
	if len(rr) != 5 || rr[1] != "X" || rr[3] != "Y" {
		t.Fatalf("LINSERT result = %v", rr)
	}
	if r := mustDo(t, cli, "LINSERT", "l", "BEFORE", "zzz", "no"); r != int64(-1) {
		t.Fatalf("LINSERT missing pivot = %v", r)
	}
	mustError(t, cli, "LINSERT", "l", "SIDEWAYS", "b", "x") // bad where -> syntax error
	mustError(t, cli, "LINSERT", "l", "BEFORE", "b")        // arity
	mustDo(t, cli, "SET", "str", "v")
	mustError(t, cli, "LINSERT", "str", "BEFORE", "a", "x") // wrong type
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

// TestCommandDoesNotCrash guards against the nil-pointer panic where COMMAND
// and COMMAND DOCS wrote an empty array with a nil callback (issue #1). redis-cli
// issues COMMAND DOCS on connect, which previously killed the server.
func TestCommandDoesNotCrash(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// Plain COMMAND and COMMAND DOCS must reply with an (empty) array, not panic.
	if r := mustDo(t, cli, "COMMAND"); r == nil {
		t.Fatalf("COMMAND returned nil")
	}
	if r := mustDo(t, cli, "COMMAND", "DOCS"); r == nil {
		t.Fatalf("COMMAND DOCS returned nil")
	}
	// COMMAND COUNT still returns the number of registered commands.
	if r := mustDo(t, cli, "COMMAND", "COUNT"); r == int64(0) {
		t.Fatalf("COMMAND COUNT = %v, want > 0", r)
	}
	// The server must still be alive and serving after those calls.
	if r := mustDo(t, cli, "PING"); r != "PONG" {
		t.Fatalf("PING after COMMAND = %v", r)
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

func TestStrLen(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "STRLEN", "nope"); r != int64(0) {
		t.Fatalf("STRLEN missing = %v, want 0", r)
	}
	mustDo(t, cli, "SET", "s", "Hello World")
	if r := mustDo(t, cli, "STRLEN", "s"); r != int64(11) {
		t.Fatalf("STRLEN = %v, want 11", r)
	}

	mustDo(t, cli, "RPUSH", "l", "a")
	r, err := cli.Do("STRLEN", "l")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.(error); !ok {
		t.Fatalf("STRLEN on a list = %v, want WRONGTYPE error", r)
	}
}

func TestGetRange(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "SET", "s", "Hello World")
	if r := mustDo(t, cli, "GETRANGE", "s", "0", "4"); r != "Hello" {
		t.Fatalf("GETRANGE 0 4 = %v, want Hello", r)
	}
	if r := mustDo(t, cli, "GETRANGE", "s", "-5", "-1"); r != "World" {
		t.Fatalf("GETRANGE -5 -1 = %v, want World", r)
	}
	if r := mustDo(t, cli, "GETRANGE", "s", "0", "-1"); r != "Hello World" {
		t.Fatalf("GETRANGE 0 -1 = %v, want Hello World", r)
	}
	if r := mustDo(t, cli, "GETRANGE", "nope", "0", "10"); r != "" {
		t.Fatalf("GETRANGE missing = %v, want empty", r)
	}
	if r := mustDo(t, cli, "GETRANGE", "s", "0", "-12"); r != "H" {
		t.Fatalf("GETRANGE 0 -12 = %v, want H", r)
	}
}

func TestSetRange(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "SET", "s", "Hello World")
	if r := mustDo(t, cli, "SETRANGE", "s", "6", "Redis"); r != int64(11) {
		t.Fatalf("SETRANGE = %v, want 11", r)
	}
	if r := mustDo(t, cli, "GET", "s"); r != "Hello Redis" {
		t.Fatalf("GET after SETRANGE = %v, want Hello Redis", r)
	}

	if r := mustDo(t, cli, "SETRANGE", "pad", "5", "hi"); r != int64(7) {
		t.Fatalf("SETRANGE pad = %v, want 7", r)
	}
	if r := mustDo(t, cli, "GET", "pad"); r != "\x00\x00\x00\x00\x00hi" {
		t.Fatalf("GET pad = %q, want 5 zero bytes then hi", r)
	}

	r, err := cli.Do("SETRANGE", "big", "9223372036854775807", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.(error); !ok {
		t.Fatalf("SETRANGE huge offset = %v, want error", r)
	}
	if r := mustDo(t, cli, "PING"); r != "PONG" {
		t.Fatalf("PING after huge SETRANGE = %v, server may have crashed", r)
	}
}

func TestExpireAt(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "EXPIREAT", "nope", "99999999999"); r != int64(0) {
		t.Fatalf("EXPIREAT missing key = %v", r)
	}

	mustDo(t, cli, "SET", "k", "v")
	if r := mustDo(t, cli, "EXPIREAT", "k", "99999999999"); r != int64(1) {
		t.Fatalf("EXPIREAT = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "k").(int64); r <= 0 {
		t.Fatalf("TTL after EXPIREAT = %v, want a positive TTL", r)
	}

	mustDo(t, cli, "SET", "past", "v")
	if r := mustDo(t, cli, "EXPIREAT", "past", "1"); r != int64(1) {
		t.Fatalf("EXPIREAT past = %v", r)
	}
	if r := mustDo(t, cli, "EXISTS", "past"); r != int64(0) {
		t.Fatalf("EXPIREAT with a past time did not delete the key = %v", r)
	}

	mustDo(t, cli, "SET", "pk", "v")
	if r := mustDo(t, cli, "PEXPIREAT", "pk", "99999999999000"); r != int64(1) {
		t.Fatalf("PEXPIREAT = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "pk").(int64); r <= 0 {
		t.Fatalf("TTL after PEXPIREAT = %v, want a positive TTL", r)
	}

	mustError(t, cli, "EXPIREAT", "k")             // arity
	mustError(t, cli, "EXPIREAT", "k", "notanint") // bad timestamp
	mustError(t, cli, "PEXPIREAT", "pk", "notanint")

	// a timestamp so large it would overflow time.Unix must be rejected, not
	// silently delete the key
	mustDo(t, cli, "SET", "keep", "v")
	mustError(t, cli, "EXPIREAT", "keep", "9223372036854775807")
	if r := mustDo(t, cli, "EXISTS", "keep"); r != int64(1) {
		t.Fatalf("EXPIREAT overflow deleted the key = %v", r)
	}
}

func TestExpireTime(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "EXPIRETIME", "nope"); r != int64(-2) {
		t.Fatalf("EXPIRETIME missing key = %v", r)
	}

	mustDo(t, cli, "SET", "k", "v")
	if r := mustDo(t, cli, "EXPIRETIME", "k"); r != int64(-1) {
		t.Fatalf("EXPIRETIME without a TTL = %v", r)
	}
	mustDo(t, cli, "EXPIREAT", "k", "99999999999")
	if r := mustDo(t, cli, "EXPIRETIME", "k"); r != int64(99999999999) {
		t.Fatalf("EXPIRETIME after EXPIREAT = %v", r)
	}

	mustDo(t, cli, "SET", "pk", "v")
	mustDo(t, cli, "PEXPIREAT", "pk", "99999999999000")
	if r := mustDo(t, cli, "EXPIRETIME", "pk"); r != int64(99999999999) {
		t.Fatalf("EXPIRETIME after PEXPIREAT (ms -> seconds) = %v", r)
	}

	mustError(t, cli, "EXPIRETIME")           // arity: no key
	mustError(t, cli, "EXPIRETIME", "k", "x") // arity: too many
}

func TestSetNX(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "SETNX", "k", "v1"); r != int64(1) {
		t.Fatalf("SETNX new key = %v", r)
	}
	if r := mustDo(t, cli, "GET", "k"); r != "v1" {
		t.Fatalf("GET after SETNX = %v", r)
	}
	if r := mustDo(t, cli, "SETNX", "k", "v2"); r != int64(0) {
		t.Fatalf("SETNX existing key = %v", r)
	}
	if r := mustDo(t, cli, "GET", "k"); r != "v1" {
		t.Fatalf("SETNX overwrote an existing key = %v", r)
	}
	// a key of another type still blocks SETNX
	mustDo(t, cli, "RPUSH", "l", "a")
	if r := mustDo(t, cli, "SETNX", "l", "v"); r != int64(0) {
		t.Fatalf("SETNX on an existing list = %v", r)
	}
	mustError(t, cli, "SETNX", "k")           // arity: too few
	mustError(t, cli, "SETNX", "k", "v", "x") // arity: too many
}

func TestSetEx(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	if r := mustDo(t, cli, "SETEX", "k", "100", "v"); r != "OK" {
		t.Fatalf("SETEX = %v", r)
	}
	if r := mustDo(t, cli, "GET", "k"); r != "v" {
		t.Fatalf("GET after SETEX = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "k").(int64); r < 1 || r > 100 {
		t.Fatalf("TTL after SETEX = %v, want 1..100", r)
	}

	if r := mustDo(t, cli, "PSETEX", "pk", "100000", "v"); r != "OK" {
		t.Fatalf("PSETEX = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "pk").(int64); r < 1 || r > 100 {
		t.Fatalf("TTL after PSETEX = %v, want 1..100", r)
	}

	// SETEX overwrites the value and resets the TTL
	mustDo(t, cli, "SET", "o", "old")
	mustDo(t, cli, "SETEX", "o", "50", "new")
	if r := mustDo(t, cli, "GET", "o"); r != "new" {
		t.Fatalf("SETEX did not overwrite = %v", r)
	}

	// a non-positive TTL is rejected
	mustError(t, cli, "SETEX", "k", "0", "v")
	mustError(t, cli, "SETEX", "k", "-1", "v")
	mustError(t, cli, "PSETEX", "k", "0", "v")
	// non-integer TTL
	mustError(t, cli, "SETEX", "k", "abc", "v")
	// a TTL so large it would overflow time.Duration is rejected, not silently dropped
	mustError(t, cli, "SETEX", "ovf", "10000000000", "v")
	if r, _ := cli.Do("GET", "ovf"); r != nil {
		t.Fatalf("SETEX overflow created a key = %v", r)
	}
	mustError(t, cli, "PSETEX", "ovf", "10000000000000", "v")
	// arity
	mustError(t, cli, "SETEX", "k", "10")
	mustError(t, cli, "PSETEX", "k", "10")
}
