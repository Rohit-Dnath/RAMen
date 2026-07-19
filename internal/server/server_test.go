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

func TestGetDel(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// missing key returns nil
	if r, err := cli.Do("GETDEL", "nope"); err != nil || r != nil {
		t.Fatalf("GETDEL missing = %v (err %v), want nil", r, err)
	}

	mustDo(t, cli, "SET", "k", "v")
	if r := mustDo(t, cli, "GETDEL", "k"); r != "v" {
		t.Fatalf("GETDEL = %v", r)
	}
	if r := mustDo(t, cli, "EXISTS", "k"); r != int64(0) {
		t.Fatalf("GETDEL did not delete the key = %v", r)
	}

	// WRONGTYPE key errors and stays
	mustDo(t, cli, "RPUSH", "lst", "x")
	mustError(t, cli, "GETDEL", "lst")
	if r := mustDo(t, cli, "EXISTS", "lst"); r != int64(1) {
		t.Fatalf("GETDEL deleted a WRONGTYPE key = %v", r)
	}

	mustError(t, cli, "GETDEL")           // arity
	mustError(t, cli, "GETDEL", "a", "b") // arity
}

func TestGetEx(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// missing key returns nil
	if r, err := cli.Do("GETEX", "nope"); err != nil || r != nil {
		t.Fatalf("GETEX missing = %v (err %v), want nil", r, err)
	}

	mustDo(t, cli, "SET", "k", "v")

	// no option behaves like GET and leaves the TTL untouched
	if r := mustDo(t, cli, "GETEX", "k"); r != "v" {
		t.Fatalf("GETEX = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "k"); r != int64(-1) {
		t.Fatalf("GETEX with no option set a TTL = %v", r)
	}

	// no option must PRESERVE an existing TTL, not clear it like PERSIST does
	mustDo(t, cli, "SET", "kt", "v")
	mustDo(t, cli, "EXPIRE", "kt", "100")
	if r := mustDo(t, cli, "GETEX", "kt"); r != "v" {
		t.Fatalf("GETEX kt = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "kt").(int64); r <= 0 || r > 100 {
		t.Fatalf("GETEX with no option changed an existing TTL = %v, want (0,100]", r)
	}

	// EX sets a TTL
	if r := mustDo(t, cli, "GETEX", "k", "EX", "100"); r != "v" {
		t.Fatalf("GETEX EX = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "k").(int64); r <= 0 || r > 100 {
		t.Fatalf("GETEX EX TTL = %v, want (0,100]", r)
	}

	// PERSIST removes it
	mustDo(t, cli, "GETEX", "k", "PERSIST")
	if r := mustDo(t, cli, "TTL", "k"); r != int64(-1) {
		t.Fatalf("GETEX PERSIST left a TTL = %v", r)
	}

	// EXAT with a past timestamp returns the value and deletes the key
	if r := mustDo(t, cli, "GETEX", "k", "EXAT", "1"); r != "v" {
		t.Fatalf("GETEX EXAT past = %v", r)
	}
	if r := mustDo(t, cli, "EXISTS", "k"); r != int64(0) {
		t.Fatalf("GETEX EXAT past did not delete the key = %v", r)
	}

	// error paths must not touch the key
	mustDo(t, cli, "SET", "k2", "v")
	mustError(t, cli, "GETEX", "k2", "EX", "0")                     // non-positive TTL
	mustError(t, cli, "GETEX", "k2", "EX", "notanint")              // bad TTL
	mustError(t, cli, "GETEX", "k2", "EX", "10000000000")           // TTL overflows time.Duration
	mustError(t, cli, "GETEX", "k2", "PX", "9999999999999")         // ms TTL overflows time.Duration
	mustError(t, cli, "GETEX", "k2", "EXAT", "9223372036854775807") // absolute ts overflow
	// a non-positive absolute expire must error and leave the key untouched;
	// unlike EXPIREAT, GETEX rejects EXAT/PXAT <= 0 rather than deleting the key
	if e := mustError(t, cli, "GETEX", "k2", "EXAT", "0"); e.Error() != "ERR invalid expire time in 'getex' command" {
		t.Fatalf("GETEX EXAT 0 error = %q", e.Error())
	}
	mustError(t, cli, "GETEX", "k2", "EXAT", "-5")
	mustError(t, cli, "GETEX", "k2", "PXAT", "0")
	mustError(t, cli, "GETEX", "k2", "BOGUS")            // unknown option
	mustError(t, cli, "GETEX", "k2", "EX")               // missing TTL value
	mustError(t, cli, "GETEX", "k2", "PERSIST", "extra") // trailing junk
	mustError(t, cli, "GETEX")                           // arity
	if r := mustDo(t, cli, "EXISTS", "k2"); r != int64(1) {
		t.Fatalf("a failed GETEX changed k2 = %v", r)
	}

	// millisecond and future-absolute paths set a TTL rather than delete
	mustDo(t, cli, "SET", "mp", "v")
	if r := mustDo(t, cli, "GETEX", "mp", "PX", "100000"); r != "v" {
		t.Fatalf("GETEX PX = %v", r)
	}
	if r := mustDo(t, cli, "PTTL", "mp").(int64); r <= 0 || r > 100000 {
		t.Fatalf("GETEX PX PTTL = %v, want (0,100000]", r)
	}
	futureMs := strconv.FormatInt(time.Now().UnixMilli()+1_000_000, 10)
	if r := mustDo(t, cli, "GETEX", "mp", "PXAT", futureMs); r != "v" {
		t.Fatalf("GETEX PXAT future = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "mp").(int64); r <= 0 {
		t.Fatalf("GETEX PXAT future did not set a TTL = %v", r)
	}
	futureSec := strconv.FormatInt(time.Now().Unix()+1000, 10)
	mustDo(t, cli, "SET", "ea", "v")
	if r := mustDo(t, cli, "GETEX", "ea", "EXAT", futureSec); r != "v" {
		t.Fatalf("GETEX EXAT future = %v", r)
	}
	if r := mustDo(t, cli, "TTL", "ea").(int64); r <= 0 {
		t.Fatalf("GETEX EXAT future did not set a TTL = %v", r)
	}

	// at most one option
	mustError(t, cli, "GETEX", "mp", "EX", "10", "PERSIST")

	// WRONGTYPE
	mustDo(t, cli, "RPUSH", "lst", "x")
	mustError(t, cli, "GETEX", "lst")
}

func TestMSetNX(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// all keys new -> 1, everything set
	if r := mustDo(t, cli, "MSETNX", "a", "1", "b", "2", "c", "3"); r != int64(1) {
		t.Fatalf("MSETNX all-new = %v, want 1", r)
	}
	if r := mustDo(t, cli, "GET", "b"); r != "2" {
		t.Fatalf("MSETNX did not set b = %v", r)
	}

	// any key already present -> 0, nothing written
	if r := mustDo(t, cli, "MSETNX", "d", "4", "a", "9", "e", "5"); r != int64(0) {
		t.Fatalf("MSETNX with an existing key = %v, want 0", r)
	}
	if r := mustDo(t, cli, "EXISTS", "d", "e"); r != int64(0) {
		t.Fatalf("MSETNX wrote keys despite an existing one = %v", r)
	}
	if r := mustDo(t, cli, "GET", "a"); r != "1" {
		t.Fatalf("MSETNX overwrote the existing key = %v", r)
	}

	// duplicate keys in one call: last value wins
	if r := mustDo(t, cli, "MSETNX", "dup", "1", "dup", "2"); r != int64(1) {
		t.Fatalf("MSETNX duplicate keys = %v, want 1", r)
	}
	if r := mustDo(t, cli, "GET", "dup"); r != "2" {
		t.Fatalf("MSETNX duplicate last-wins = %v", r)
	}

	mustError(t, cli, "MSETNX", "k")            // no value for the key
	mustError(t, cli, "MSETNX", "k", "v", "k2") // dangling key
	mustError(t, cli, "MSETNX")                 // nothing
}

// scanReply asserts a single SCAN-family reply is a two-element array and
// returns the next cursor plus the elements.
func scanReply(t *testing.T, r any) (string, []string) {
	t.Helper()
	arr, ok := r.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("scan reply is not a two-element array: %v", r)
	}
	cursor, ok := arr[0].(string)
	if !ok {
		t.Fatalf("scan cursor is not a bulk string: %v", arr[0])
	}
	rawElems, ok := arr[1].([]any)
	if !ok {
		t.Fatalf("scan elements is not an array: %v", arr[1])
	}
	elems := make([]string, len(rawElems))
	for i, e := range rawElems {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("scan element %d is not a string: %v", i, e)
		}
		elems[i] = s
	}
	return cursor, elems
}

// scanCollect drives a SCAN-family command to completion and returns every
// element gathered across the pages. prefix is the command plus any key that
// comes before the cursor (e.g. {"SCAN"} or {"HSCAN", "h"}); match and count
// are optional (empty/zero to omit). It fails if the cursor never returns to 0.
func scanCollect(t *testing.T, cli *client.Client, prefix []string, match string, count int) []string {
	t.Helper()
	cursor := "0"
	var out []string
	for iter := 0; ; iter++ {
		if iter > 200000 {
			t.Fatalf("%v scan did not terminate (cursor stuck at %s)", prefix, cursor)
		}
		args := append([]string{}, prefix...)
		args = append(args, cursor)
		if match != "" {
			args = append(args, "MATCH", match)
		}
		if count > 0 {
			args = append(args, "COUNT", strconv.Itoa(count))
		}
		next, elems := scanReply(t, mustDo(t, cli, args...))
		out = append(out, elems...)
		if next == "0" {
			return out
		}
		cursor = next
	}
}

func TestScan(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// An empty keyspace terminates immediately at cursor 0 with no elements.
	if cursor, elems := scanReply(t, mustDo(t, cli, "SCAN", "0")); cursor != "0" || len(elems) != 0 {
		t.Fatalf("SCAN on empty db = (%q, %v)", cursor, elems)
	}

	// Insert enough keys to span many shards and force multiple pages.
	const n = 1000
	want := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		k := "key:" + strconv.Itoa(i)
		mustDo(t, cli, "SET", k, "v")
		want[k] = true
	}

	// A full scan visits every key exactly once, regardless of COUNT.
	for _, count := range []int{0, 1, 7, 100} {
		got := scanCollect(t, cli, []string{"SCAN"}, "", count)
		seen := make(map[string]bool, len(got))
		for _, k := range got {
			if seen[k] {
				t.Fatalf("COUNT %d: key %q returned twice", count, k)
			}
			seen[k] = true
		}
		if len(seen) != n {
			t.Fatalf("COUNT %d: scanned %d distinct keys, want %d", count, len(seen), n)
		}
		for k := range want {
			if !seen[k] {
				t.Fatalf("COUNT %d: key %q was missed", count, k)
			}
		}
	}

	// MATCH filters the results: "key:1?" matches key:10..key:19.
	got := scanCollect(t, cli, []string{"SCAN"}, "key:1?", 0)
	if len(got) != 10 {
		t.Fatalf("SCAN MATCH key:1? returned %d keys, want 10: %v", len(got), got)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("SCAN MATCH returned an unexpected key %q", k)
		}
	}

	// MATCH is applied after COUNT keys are fetched, so a page can be empty with
	// a non-zero cursor. A client must loop until the cursor is 0, not stop at
	// the first empty page. Assert an empty non-terminal page occurs and the full
	// loop still matches nothing.
	sawEmptyNonTerminal := false
	cursor := "0"
	for {
		next, elems := scanReply(t, mustDo(t, cli, "SCAN", cursor, "MATCH", "nomatch:*", "COUNT", "10"))
		if next != "0" && len(elems) == 0 {
			sawEmptyNonTerminal = true
		}
		if len(elems) != 0 {
			t.Fatalf("SCAN MATCH nomatch:* returned elements: %v", elems)
		}
		if next == "0" {
			break
		}
		cursor = next
	}
	if !sawEmptyNonTerminal {
		t.Fatal("expected at least one empty page with a non-zero cursor")
	}

	// Error handling: a bad cursor, a non-integer or non-positive COUNT, a
	// dangling MATCH/COUNT, and an unknown option are all rejected.
	mustError(t, cli, "SCAN", "notanumber")
	mustError(t, cli, "SCAN", "0", "COUNT", "abc")
	mustError(t, cli, "SCAN", "0", "COUNT", "0")
	mustError(t, cli, "SCAN", "0", "COUNT", "-1")
	mustError(t, cli, "SCAN", "0", "MATCH")
	mustError(t, cli, "SCAN", "0", "COUNT")
	mustError(t, cli, "SCAN", "0", "BOGUS", "x")
	mustError(t, cli, "SCAN")

	// A wildly out-of-range cursor must not panic; it yields a valid reply and
	// the iteration still terminates cleanly at 0.
	cursor = "999999999999999999"
	for iter := 0; ; iter++ {
		if iter > 1000 {
			t.Fatal("huge-cursor scan did not terminate")
		}
		next, _ := scanReply(t, mustDo(t, cli, "SCAN", cursor))
		if next == "0" {
			break
		}
		cursor = next
	}
}

// flatToMap turns a flat [k, v, k, v, ...] scan page into a map, failing on an
// odd length or a repeated key (a field/member must not appear twice in a scan
// with no concurrent mutation).
func flatToMap(t *testing.T, flat []string) map[string]string {
	t.Helper()
	if len(flat)%2 != 0 {
		t.Fatalf("flat list has odd length %d: %v", len(flat), flat)
	}
	m := make(map[string]string, len(flat)/2)
	for i := 0; i < len(flat); i += 2 {
		if _, dup := m[flat[i]]; dup {
			t.Fatalf("key %q returned twice: %v", flat[i], flat)
		}
		m[flat[i]] = flat[i+1]
	}
	return m
}

func TestHScan(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// Missing key: cursor 0, no elements.
	if cursor, elems := scanReply(t, mustDo(t, cli, "HSCAN", "nope", "0")); cursor != "0" || len(elems) != 0 {
		t.Fatalf("HSCAN missing = (%q, %v)", cursor, elems)
	}

	const n = 500
	want := make(map[string]string, n)
	for i := 0; i < n; i++ {
		f := "f:" + strconv.Itoa(i)
		v := "v:" + strconv.Itoa(i)
		mustDo(t, cli, "HSET", "h", f, v)
		want[f] = v
	}

	// A full scan returns every field/value pair exactly once, for any COUNT.
	for _, count := range []int{0, 1, 13, 1000} {
		got := flatToMap(t, scanCollect(t, cli, []string{"HSCAN", "h"}, "", count))
		if len(got) != n {
			t.Fatalf("COUNT %d: got %d pairs, want %d", count, len(got), n)
		}
		for f, v := range want {
			if got[f] != v {
				t.Fatalf("COUNT %d: field %q = %q, want %q", count, f, got[f], v)
			}
		}
	}

	// MATCH filters on the field name: "f:1?" matches f:10..f:19.
	got := flatToMap(t, scanCollect(t, cli, []string{"HSCAN", "h"}, "f:1?", 0))
	if len(got) != 10 {
		t.Fatalf("HSCAN MATCH f:1? returned %d pairs, want 10: %v", len(got), got)
	}
	for f, v := range got {
		if want[f] != v {
			t.Fatalf("HSCAN MATCH returned unexpected pair %q=%q", f, v)
		}
	}

	// MATCH filters on the field name, not the value: a value-shaped pattern
	// matches nothing even though values look like "v:NN".
	if got := scanCollect(t, cli, []string{"HSCAN", "h"}, "v:1?", 0); len(got) != 0 {
		t.Fatalf("HSCAN MATCH v:1? matched values, got %v", got)
	}

	// Wrong type and arity are rejected.
	mustDo(t, cli, "SET", "str", "x")
	mustError(t, cli, "HSCAN", "str", "0")
	mustError(t, cli, "HSCAN", "h")
	mustError(t, cli, "HSCAN", "h", "notanumber")
	mustError(t, cli, "HSCAN", "h", "0", "COUNT", "0")
}

func TestSScan(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// Missing key: cursor 0, no elements.
	if cursor, elems := scanReply(t, mustDo(t, cli, "SSCAN", "nope", "0")); cursor != "0" || len(elems) != 0 {
		t.Fatalf("SSCAN missing = (%q, %v)", cursor, elems)
	}

	const n = 500
	want := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		m := "m:" + strconv.Itoa(i)
		mustDo(t, cli, "SADD", "s", m)
		want[m] = true
	}

	// A full scan returns every member exactly once, for any COUNT.
	for _, count := range []int{0, 1, 13, 1000} {
		got := scanCollect(t, cli, []string{"SSCAN", "s"}, "", count)
		seen := make(map[string]bool, len(got))
		for _, m := range got {
			if seen[m] {
				t.Fatalf("COUNT %d: member %q returned twice", count, m)
			}
			seen[m] = true
		}
		if len(seen) != n {
			t.Fatalf("COUNT %d: got %d members, want %d", count, len(seen), n)
		}
		for m := range want {
			if !seen[m] {
				t.Fatalf("COUNT %d: member %q missed", count, m)
			}
		}
	}

	// MATCH filters on the member: "m:1?" matches m:10..m:19.
	got := scanCollect(t, cli, []string{"SSCAN", "s"}, "m:1?", 0)
	if len(got) != 10 {
		t.Fatalf("SSCAN MATCH m:1? returned %d members, want 10: %v", len(got), got)
	}

	// Wrong type and arity are rejected.
	mustDo(t, cli, "SET", "str", "x")
	mustError(t, cli, "SSCAN", "str", "0")
	mustError(t, cli, "SSCAN", "s")
	mustError(t, cli, "SSCAN", "s", "notanumber")
}

func TestZScan(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	// Missing key: cursor 0, no elements.
	if cursor, elems := scanReply(t, mustDo(t, cli, "ZSCAN", "nope", "0")); cursor != "0" || len(elems) != 0 {
		t.Fatalf("ZSCAN missing = (%q, %v)", cursor, elems)
	}

	const n = 400
	want := make(map[string]string, n)
	for i := 0; i < n; i++ {
		m := "m:" + strconv.Itoa(i)
		mustDo(t, cli, "ZADD", "z", strconv.Itoa(i), m)
		want[m] = strconv.Itoa(i) // formatFloat of an integer-valued score
	}

	// A full scan returns every member/score pair exactly once, for any COUNT.
	for _, count := range []int{0, 1, 13, 1000} {
		got := flatToMap(t, scanCollect(t, cli, []string{"ZSCAN", "z"}, "", count))
		if len(got) != n {
			t.Fatalf("COUNT %d: got %d pairs, want %d", count, len(got), n)
		}
		for m, sc := range want {
			if got[m] != sc {
				t.Fatalf("COUNT %d: member %q score = %q, want %q", count, m, got[m], sc)
			}
		}
	}

	// MATCH filters on the member name: "m:1?" matches m:10..m:19.
	got := flatToMap(t, scanCollect(t, cli, []string{"ZSCAN", "z"}, "m:1?", 0))
	if len(got) != 10 {
		t.Fatalf("ZSCAN MATCH m:1? returned %d pairs, want 10: %v", len(got), got)
	}
	for m, sc := range got {
		if want[m] != sc {
			t.Fatalf("ZSCAN MATCH returned unexpected pair %q=%q", m, sc)
		}
	}

	// MATCH filters on the member, not the score: a score-shaped pattern
	// matches nothing.
	if got := scanCollect(t, cli, []string{"ZSCAN", "z"}, "1?", 0); len(got) != 0 {
		t.Fatalf("ZSCAN MATCH 1? matched scores, got %v", got)
	}

	// Scores are formatted like ZSCORE, including fractional values.
	mustDo(t, cli, "ZADD", "z2", "3.5", "half")
	_, elems := scanReply(t, mustDo(t, cli, "ZSCAN", "z2", "0"))
	if len(elems) != 2 || elems[0] != "half" || elems[1] != "3.5" {
		t.Fatalf("ZSCAN score formatting = %v", elems)
	}

	// Wrong type and arity are rejected.
	mustDo(t, cli, "SET", "str", "x")
	mustError(t, cli, "ZSCAN", "str", "0")
	mustError(t, cli, "ZSCAN", "z")
	mustError(t, cli, "ZSCAN", "z", "notanumber")
}

// TestScanCrashResistance covers hostile inputs that must not panic the server:
// a COUNT large enough to overflow the slice index, and a cursor past the end of
// the collection. Both must return a valid reply and keep the server serving.
func TestScanCrashResistance(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "HSET", "h", "a", "1", "b", "2")
	mustDo(t, cli, "SADD", "s", "x", "y")
	mustDo(t, cli, "ZADD", "z", "1", "p", "2", "q")

	const huge = "9223372036854775807" // math.MaxInt64
	const past = "999999999999999999"

	// A huge COUNT starting mid-collection must return the tail and cursor 0,
	// not overflow the slice bound and crash.
	if cur, elems := scanReply(t, mustDo(t, cli, "HSCAN", "h", "1", "COUNT", huge)); cur != "0" || len(elems) != 2 {
		t.Fatalf("HSCAN huge COUNT = (%q, %v)", cur, elems)
	}
	if cur, elems := scanReply(t, mustDo(t, cli, "SSCAN", "s", "1", "COUNT", huge)); cur != "0" || len(elems) != 1 {
		t.Fatalf("SSCAN huge COUNT = (%q, %v)", cur, elems)
	}
	if cur, elems := scanReply(t, mustDo(t, cli, "ZSCAN", "z", "1", "COUNT", huge)); cur != "0" || len(elems) != 2 {
		t.Fatalf("ZSCAN huge COUNT = (%q, %v)", cur, elems)
	}

	// An out-of-range cursor must terminate cleanly at 0 with no elements.
	for _, cmd := range [][]string{{"HSCAN", "h"}, {"SSCAN", "s"}, {"ZSCAN", "z"}} {
		args := append(append([]string{}, cmd...), past)
		if cur, elems := scanReply(t, mustDo(t, cli, args...)); cur != "0" || len(elems) != 0 {
			t.Fatalf("%v out-of-range cursor = (%q, %v)", cmd, cur, elems)
		}
	}

	// The server is still responsive after all of the above.
	if r := mustDo(t, cli, "PING"); r != "PONG" {
		t.Fatalf("server unresponsive after hostile scans: %v", r)
	}
}

// TestScanSkipsExpired verifies a scan never returns a key whose TTL has passed,
// and that a typed scan of an expired collection is treated as a missing key.
func TestScanSkipsExpired(t *testing.T) {
	cli, cleanup := startTestServer(t)
	defer cleanup()

	mustDo(t, cli, "SET", "keep", "v")
	mustDo(t, cli, "SET", "gone", "v", "PX", "20")
	mustDo(t, cli, "HSET", "h", "f", "v")
	mustDo(t, cli, "PEXPIRE", "h", "20")
	time.Sleep(60 * time.Millisecond)

	keys := scanCollect(t, cli, []string{"SCAN"}, "", 0)
	for _, k := range keys {
		if k == "gone" {
			t.Fatalf("SCAN returned the expired key %q", k)
		}
	}
	found := false
	for _, k := range keys {
		if k == "keep" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SCAN dropped the live key: %v", keys)
	}

	// The expired hash scans like a missing key.
	if cur, elems := scanReply(t, mustDo(t, cli, "HSCAN", "h", "0")); cur != "0" || len(elems) != 0 {
		t.Fatalf("HSCAN of expired hash = (%q, %v)", cur, elems)
	}
}
