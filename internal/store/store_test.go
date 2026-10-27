package store

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStringSetGet(t *testing.T) {
	s := New()
	s.Set("k", "v", SetOptions{})
	got, ok, err := s.Get("k")
	if err != nil || !ok || got != "v" {
		t.Fatalf("Get = %q %v %v", got, ok, err)
	}
}

func TestSetNX(t *testing.T) {
	s := New()
	if !s.Set("k", "v1", SetOptions{NX: true}) {
		t.Fatal("first NX set should succeed")
	}
	if s.Set("k", "v2", SetOptions{NX: true}) {
		t.Fatal("second NX set should fail")
	}
	got, _, _ := s.Get("k")
	if got != "v1" {
		t.Fatalf("got %q", got)
	}
}

func TestIncrBy(t *testing.T) {
	s := New()
	n, err := s.IncrBy("c", 5)
	if err != nil || n != 5 {
		t.Fatalf("IncrBy = %d %v", n, err)
	}
	n, _ = s.IncrBy("c", -2)
	if n != 3 {
		t.Fatalf("got %d", n)
	}
}

func TestIncrByFloat(t *testing.T) {
	s := New()
	v, err := s.IncrByFloat("c", 1.5)
	if err != nil || v != "1.5" {
		t.Fatalf("IncrByFloat create = %q %v", v, err)
	}
	v, err = s.IncrByFloat("c", 2.25)
	if err != nil || v != "3.75" {
		t.Fatalf("IncrByFloat existing = %q %v", v, err)
	}
	v, err = s.IncrByFloat("c", -3.75)
	if err != nil || v != "0" {
		t.Fatalf("IncrByFloat zero normalization = %q %v", v, err)
	}
	v, err = s.IncrByFloat("whole", 5)
	if err != nil || v != "5" {
		t.Fatalf("IncrByFloat integer-valued result = %q %v", v, err)
	}
	s.Set("neg", "-0", SetOptions{})
	v, err = s.IncrByFloat("neg", math.Copysign(0, -1))
	if err != nil || v != "0" {
		t.Fatalf("IncrByFloat negative-zero normalization = %q %v", v, err)
	}
	s.Set("sci", "1e3", SetOptions{})
	v, err = s.IncrByFloat("sci", 0.5)
	if err != nil || v != "1000.5" {
		t.Fatalf("IncrByFloat scientific value = %q %v", v, err)
	}
	s.Set("bad", "abc", SetOptions{})
	if _, err := s.IncrByFloat("bad", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat non-float value = %v", err)
	}
	if got, _, _ := s.Get("bad"); got != "abc" {
		t.Fatalf("IncrByFloat changed bad value = %q", got)
	}
	s.Set("nan", "NaN", SetOptions{})
	if _, err := s.IncrByFloat("nan", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat NaN value = %v", err)
	}
	if _, err := s.IncrByFloat("d", math.Inf(1)); err != ErrNotFloat {
		t.Fatalf("IncrByFloat infinite delta = %v", err)
	}
	if _, ok, _ := s.Get("d"); ok {
		t.Fatalf("IncrByFloat wrote a key for an infinite delta")
	}
	s.Set("huge", "1e308", SetOptions{})
	if _, err := s.IncrByFloat("huge", 1e308); err != ErrFloatOverflow {
		t.Fatalf("IncrByFloat overflow = %v", err)
	}
	if got, _, _ := s.Get("huge"); got != "1e308" {
		t.Fatalf("IncrByFloat changed overflow value = %q", got)
	}
	s.Set("inf", "inf", SetOptions{})
	if _, err := s.IncrByFloat("inf", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat infinite stored value = %v", err)
	}
	s.Set("ttl", "1", SetOptions{})
	s.Expire("ttl", time.Minute)
	if _, err := s.IncrByFloat("ttl", 1.5); err != nil {
		t.Fatalf("IncrByFloat on a key with TTL = %v", err)
	}
	if _, hasTTL, ok := s.TTL("ttl"); !ok || !hasTTL {
		t.Fatalf("IncrByFloat dropped the key's TTL: ok=%v hasTTL=%v", ok, hasTTL)
	}
	s.LPush("l", "a")
	if _, err := s.IncrByFloat("l", 1.5); err != ErrWrongType {
		t.Fatalf("IncrByFloat wrong type = %v", err)
	}
}

func TestHashSetNX(t *testing.T) {
	s := New()
	ok, err := s.HSetNX("h", "f", "v1")
	if err != nil || !ok {
		t.Fatalf("HSetNX create = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", "f"); err != nil || !found || v != "v1" {
		t.Fatalf("HGet after HSetNX = %q %v %v", v, found, err)
	}
	ok, err = s.HSetNX("h", "f", "v2")
	if err != nil || ok {
		t.Fatalf("HSetNX existing = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", "f"); err != nil || !found || v != "v1" {
		t.Fatalf("HSetNX overwrote existing field: %q %v %v", v, found, err)
	}
	ok, err = s.HSetNX("h", "", "")
	if err != nil || !ok {
		t.Fatalf("HSetNX empty field/value = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", ""); err != nil || !found || v != "" {
		t.Fatalf("HGet empty field = %q %v %v", v, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HSetNX("str", "f", "v"); err != ErrWrongType {
		t.Fatalf("HSetNX wrong type = %v", err)
	}
}

func TestHashIncrBy(t *testing.T) {
	s := New()
	n, err := s.HIncrBy("h", "count", 5)
	if err != nil || n != 5 {
		t.Fatalf("HIncrBy create = %d %v", n, err)
	}
	if v, found, err := s.HGet("h", "count"); err != nil || !found || v != "5" {
		t.Fatalf("HGet after HIncrBy create = %q %v %v", v, found, err)
	}
	n, err = s.HIncrBy("h", "count", -2)
	if err != nil || n != 3 {
		t.Fatalf("HIncrBy existing = %d %v", n, err)
	}
	if v, found, err := s.HGet("h", "count"); err != nil || !found || v != "3" {
		t.Fatalf("HGet after HIncrBy existing = %q %v %v", v, found, err)
	}
	s.HSet("h", map[string]string{"bad": "abc"})
	if _, err := s.HIncrBy("h", "bad", 1); err != ErrHashNotInteger {
		t.Fatalf("HIncrBy non-integer field = %v", err)
	}
	if v, found, err := s.HGet("h", "bad"); err != nil || !found || v != "abc" {
		t.Fatalf("HIncrBy changed bad field = %q %v %v", v, found, err)
	}
	max := strconv.FormatInt(math.MaxInt64, 10)
	s.HSet("h", map[string]string{"max": max})
	if _, err := s.HIncrBy("h", "max", 1); err != ErrIntegerOverflow {
		t.Fatalf("HIncrBy overflow = %v", err)
	}
	if v, found, err := s.HGet("h", "max"); err != nil || !found || v != max {
		t.Fatalf("HIncrBy changed overflow field = %q %v %v", v, found, err)
	}
	min := strconv.FormatInt(math.MinInt64, 10)
	s.HSet("h", map[string]string{"min": min})
	if _, err := s.HIncrBy("h", "min", -1); err != ErrIntegerOverflow {
		t.Fatalf("HIncrBy underflow = %v", err)
	}
	if v, found, err := s.HGet("h", "min"); err != nil || !found || v != min {
		t.Fatalf("HIncrBy changed underflow field = %q %v %v", v, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HIncrBy("str", "f", 1); err != ErrWrongType {
		t.Fatalf("HIncrBy wrong type = %v", err)
	}
}

func TestHashIncrByFloat(t *testing.T) {
	s := New()
	v, err := s.HIncrByFloat("h", "score", 1.5)
	if err != nil || v != "1.5" {
		t.Fatalf("HIncrByFloat create = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "score", 2.25)
	if err != nil || v != "3.75" {
		t.Fatalf("HIncrByFloat existing = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "score", -3.75)
	if err != nil || v != "0" {
		t.Fatalf("HIncrByFloat zero normalization = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "whole", 5)
	if err != nil || v != "5" {
		t.Fatalf("HIncrByFloat integer-valued result = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"neg": "-0"})
	v, err = s.HIncrByFloat("h", "neg", math.Copysign(0, -1))
	if err != nil || v != "0" {
		t.Fatalf("HIncrByFloat negative-zero normalization = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"sci": "1e3"})
	v, err = s.HIncrByFloat("h", "sci", 0.5)
	if err != nil || v != "1000.5" {
		t.Fatalf("HIncrByFloat scientific field = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"bad": "abc", "nan": "NaN"})
	if _, err := s.HIncrByFloat("h", "bad", 1); err != ErrHashNotFloat {
		t.Fatalf("HIncrByFloat bad field = %v", err)
	}
	if _, err := s.HIncrByFloat("h", "nan", 1); err != ErrHashNotFloat {
		t.Fatalf("HIncrByFloat NaN field = %v", err)
	}
	if _, err := s.HIncrByFloat("h", "inf-delta", math.Inf(1)); err != ErrNotFloat {
		t.Fatalf("HIncrByFloat infinite delta = %v", err)
	}
	if _, found, err := s.HGet("h", "inf-delta"); err != nil || found {
		t.Fatalf("HIncrByFloat wrote infinite delta field = %v %v", found, err)
	}
	s.HSet("h", map[string]string{"huge": "1e308"})
	if _, err := s.HIncrByFloat("h", "huge", 1e308); err != ErrFloatOverflow {
		t.Fatalf("HIncrByFloat overflow = %v", err)
	}
	if got, found, err := s.HGet("h", "huge"); err != nil || !found || got != "1e308" {
		t.Fatalf("HIncrByFloat changed overflow field = %q %v %v", got, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HIncrByFloat("str", "f", 1.5); err != ErrWrongType {
		t.Fatalf("HIncrByFloat wrong type = %v", err)
	}
}

func TestWrongType(t *testing.T) {
	s := New()
	s.LPush("l", "a")
	if _, _, err := s.Get("l"); err != ErrWrongType {
		t.Fatalf("want ErrWrongType, got %v", err)
	}
}

func TestExpiryLazy(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }
	s.Set("k", "v", SetOptions{})
	s.Expire("k", time.Second)
	cur = cur.Add(2 * time.Second)
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("key should have lazily expired")
	}
}

func TestExpireAt(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if s.ExpireAt("nope", time.Unix(2000, 0)) {
		t.Fatal("ExpireAt on a missing key should be false")
	}

	s.Set("k", "v", SetOptions{})
	if !s.ExpireAt("k", time.Unix(2000, 0)) {
		t.Fatal("ExpireAt with a future deadline should be true")
	}
	if _, ok, _ := s.Get("k"); !ok {
		t.Fatal("key should still be alive before its deadline")
	}
	cur = time.Unix(2001, 0)
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("key should be gone once the deadline passes")
	}

	s.Set("past", "v", SetOptions{})
	if !s.ExpireAt("past", time.Unix(500, 0)) {
		t.Fatal("ExpireAt with a past deadline should still return true")
	}
	if s.Exists("past") != 0 {
		t.Fatal("ExpireAt with a past deadline should delete the key")
	}
}

func TestExpireTime(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if _, _, ok := s.ExpireTime("nope"); ok {
		t.Fatal("ExpireTime on a missing key should be ok=false")
	}

	s.Set("k", "v", SetOptions{})
	if _, hasTTL, ok := s.ExpireTime("k"); !ok || hasTTL {
		t.Fatalf("ExpireTime without a TTL = hasTTL=%v ok=%v", hasTTL, ok)
	}

	s.ExpireAt("k", time.Unix(5000, 0))
	if at, hasTTL, ok := s.ExpireTime("k"); !ok || !hasTTL || at.Unix() != 5000 {
		t.Fatalf("ExpireTime = %d hasTTL=%v ok=%v", at.Unix(), hasTTL, ok)
	}
}

func TestSnapshotRoundtrip(t *testing.T) {
	s := New()
	s.Set("str", "hello", SetOptions{})
	s.HSet("h", map[string]string{"f": "v"})
	s.RPush("l", "a", "b")
	s.SAdd("set", "x", "y")
	s.ZAdd("z", []ZMember{{Member: "m", Score: 1.5}})
	s.VSet("vec", "id1", []float32{1, 2, 3}, "meta", 0)

	recs := s.Export()
	s2 := New()
	s2.Import(recs)

	if v, _, _ := s2.Get("str"); v != "hello" {
		t.Fatalf("str = %q", v)
	}
	if v, _, _ := s2.HGet("h", "f"); v != "v" {
		t.Fatalf("hash = %q", v)
	}
	if n, _ := s2.LLen("l"); n != 2 {
		t.Fatalf("list len = %d", n)
	}
	if n, _ := s2.SCard("set"); n != 2 {
		t.Fatalf("set card = %d", n)
	}
	if sc, ok, _ := s2.ZScore("z", "m"); !ok || sc != 1.5 {
		t.Fatalf("zscore = %f %v", sc, ok)
	}
	if n, _ := s2.VCard("vec"); n != 1 {
		t.Fatalf("vcard = %d", n)
	}
}

func TestListSet(t *testing.T) {
	s := New()
	if err := s.LSet("l", 0, "x"); err != ErrNoSuchKey {
		t.Fatalf("LSet missing key = %v", err)
	}
	s.RPush("l", "a", "b", "c")
	if err := s.LSet("l", 1, "B"); err != nil {
		t.Fatalf("LSet = %v", err)
	}
	if v, _, _ := s.LIndex("l", 1); v != "B" {
		t.Fatalf("LSet did not update index 1 = %q", v)
	}
	if err := s.LSet("l", -1, "C"); err != nil {
		t.Fatalf("LSet negative index = %v", err)
	}
	if v, _, _ := s.LIndex("l", 2); v != "C" {
		t.Fatalf("LSet negative index did not update tail = %q", v)
	}
	if err := s.LSet("l", 3, "z"); err != ErrIndexOutOfRange {
		t.Fatalf("LSet out of range = %v", err)
	}
	if err := s.LSet("l", -4, "z"); err != ErrIndexOutOfRange {
		t.Fatalf("LSet negative out of range = %v", err)
	}
	s.Set("str", "value", SetOptions{})
	if err := s.LSet("str", 0, "x"); err != ErrWrongType {
		t.Fatalf("LSet wrong type = %v", err)
	}
}

func TestListRem(t *testing.T) {
	s := New()
	if n, err := s.LRem("nope", 0, "a"); err != nil || n != 0 {
		t.Fatalf("LRem missing key = %d %v", n, err)
	}

	s.RPush("pos", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("pos", 2, "a"); n != 2 {
		t.Fatalf("LRem count>0 removed = %d", n)
	}
	if got, _ := s.LRange("pos", 0, -1); strings.Join(got, ",") != "b,c,a" {
		t.Fatalf("LRem count>0 result = %v", got)
	}

	s.RPush("neg", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("neg", -2, "a"); n != 2 {
		t.Fatalf("LRem count<0 removed = %d", n)
	}
	if got, _ := s.LRange("neg", 0, -1); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("LRem count<0 result = %v", got)
	}

	s.RPush("all", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("all", 0, "a"); n != 3 {
		t.Fatalf("LRem count==0 removed = %d", n)
	}
	if got, _ := s.LRange("all", 0, -1); strings.Join(got, ",") != "b,c" {
		t.Fatalf("LRem count==0 result = %v", got)
	}
	if n, _ := s.LRem("all", 0, "zzz"); n != 0 {
		t.Fatalf("LRem no match = %d", n)
	}

	s.RPush("empty", "x", "x")
	s.LRem("empty", 0, "x")
	if s.Exists("empty") != 0 {
		t.Fatalf("LRem did not drop the emptied key")
	}

	// int64-min count: -count overflows, must still remove all matches like Redis
	s.RPush("min", "a", "b", "a")
	if n, _ := s.LRem("min", math.MinInt64, "a"); n != 2 {
		t.Fatalf("LRem int64-min count removed = %d", n)
	}
	if got, _ := s.LRange("min", 0, -1); strings.Join(got, ",") != "b" {
		t.Fatalf("LRem int64-min count result = %v", got)
	}

	s.Set("str", "v", SetOptions{})
	if _, err := s.LRem("str", 0, "x"); err != ErrWrongType {
		t.Fatalf("LRem wrong type = %v", err)
	}
}

func TestListTrim(t *testing.T) {
	s := New()
	if err := s.LTrim("nope", 0, -1); err != nil {
		t.Fatalf("LTrim missing key = %v", err)
	}

	s.RPush("l", "a", "b", "c", "d", "e")
	if err := s.LTrim("l", 1, 3); err != nil {
		t.Fatalf("LTrim = %v", err)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "b,c,d" {
		t.Fatalf("LTrim result = %v", got)
	}

	s.RPush("neg", "a", "b", "c", "d", "e")
	s.LTrim("neg", -3, -1)
	if got, _ := s.LRange("neg", 0, -1); strings.Join(got, ",") != "c,d,e" {
		t.Fatalf("LTrim negative indices = %v", got)
	}

	s.RPush("clamp", "a", "b", "c")
	s.LTrim("clamp", -100, 100)
	if got, _ := s.LRange("clamp", 0, -1); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("LTrim clamp = %v", got)
	}

	s.RPush("past", "a", "b", "c")
	s.LTrim("past", 5, 10)
	if s.Exists("past") != 0 {
		t.Fatalf("LTrim start past the end did not drop the key")
	}
	s.RPush("rev", "a", "b", "c")
	s.LTrim("rev", 2, 1)
	if s.Exists("rev") != 0 {
		t.Fatalf("LTrim start>stop did not drop the key")
	}

	s.Set("str", "v", SetOptions{})
	if err := s.LTrim("str", 0, -1); err != ErrWrongType {
		t.Fatalf("LTrim wrong type = %v", err)
	}
}

func TestListInsert(t *testing.T) {
	s := New()
	if n, err := s.LInsert("nope", true, "a", "x"); err != nil || n != 0 {
		t.Fatalf("LInsert missing key = %d %v", n, err)
	}

	s.RPush("l", "a", "b", "c")
	if n, _ := s.LInsert("l", true, "b", "X"); n != 4 {
		t.Fatalf("LInsert before length = %d", n)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "a,X,b,c" {
		t.Fatalf("LInsert before result = %v", got)
	}
	if n, _ := s.LInsert("l", false, "b", "Y"); n != 5 {
		t.Fatalf("LInsert after length = %d", n)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "a,X,b,Y,c" {
		t.Fatalf("LInsert after result = %v", got)
	}

	if n, _ := s.LInsert("l", true, "a", "HEAD"); n != 6 {
		t.Fatalf("LInsert before head length = %d", n)
	}
	if got, _ := s.LRange("l", 0, 0); got[0] != "HEAD" {
		t.Fatalf("LInsert before head result = %v", got)
	}
	if n, _ := s.LInsert("l", false, "c", "TAIL"); n != 7 {
		t.Fatalf("LInsert after tail length = %d", n)
	}
	if got, _ := s.LRange("l", -1, -1); got[0] != "TAIL" {
		t.Fatalf("LInsert after tail result = %v", got)
	}

	if n, _ := s.LInsert("l", true, "zzz", "no"); n != -1 {
		t.Fatalf("LInsert missing pivot = %d", n)
	}
	if n, _ := s.LLen("l"); n != 7 {
		t.Fatalf("LInsert missing pivot changed length = %d", n)
	}

	s.RPush("dup", "a", "b", "a")
	s.LInsert("dup", true, "a", "Z")
	if got, _ := s.LRange("dup", 0, -1); strings.Join(got, ",") != "Z,a,b,a" {
		t.Fatalf("LInsert first-occurrence only = %v", got)
	}

	s.Set("str", "v", SetOptions{})
	if _, err := s.LInsert("str", true, "a", "x"); err != ErrWrongType {
		t.Fatalf("LInsert wrong type = %v", err)
	}
}

func TestGetDel(t *testing.T) {
	s := New()

	// missing key: not found, nothing deleted
	if _, ok, err := s.GetDel("nope"); ok || err != nil {
		t.Fatalf("GetDel missing = ok=%v err=%v", ok, err)
	}

	// get-and-delete
	s.Set("k", "v", SetOptions{})
	if v, ok, err := s.GetDel("k"); err != nil || !ok || v != "v" {
		t.Fatalf("GetDel = %q ok=%v err=%v", v, ok, err)
	}
	if s.Exists("k") != 0 {
		t.Fatal("GetDel did not delete the key")
	}

	// a WRONGTYPE key errors and is left in place
	s.push("lst", true, []string{"x"})
	if _, _, err := s.GetDel("lst"); err != ErrWrongType {
		t.Fatalf("GetDel wrong type = %v, want ErrWrongType", err)
	}
	if s.Exists("lst") != 1 {
		t.Fatal("GetDel deleted a WRONGTYPE key")
	}
}

func TestGetEx(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if _, ok, err := s.GetEx("nope", GetExOp{}); ok || err != nil {
		t.Fatalf("GetEx missing = ok=%v err=%v", ok, err)
	}

	// no change leaves an existing TTL untouched
	s.Set("k", "v", SetOptions{})
	s.Expire("k", 100*time.Second) // deadline at 1100
	if v, ok, err := s.GetEx("k", GetExOp{Mode: GetExNoChange}); err != nil || !ok || v != "v" {
		t.Fatalf("GetEx no-change = %q ok=%v err=%v", v, ok, err)
	}
	if at, hasTTL, _ := s.ExpireTime("k"); !hasTTL || at.Unix() != 1100 {
		t.Fatalf("GetEx no-change altered the TTL: at=%d hasTTL=%v", at.Unix(), hasTTL)
	}

	// relative TTL
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExSetTTL, TTL: 50 * time.Second}); !ok {
		t.Fatal("GetEx set-ttl not ok")
	}
	if at, _, _ := s.ExpireTime("k"); at.Unix() != 1050 {
		t.Fatalf("GetEx set-ttl deadline = %d, want 1050", at.Unix())
	}

	// persist removes the TTL
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExPersist}); !ok {
		t.Fatal("GetEx persist not ok")
	}
	if _, hasTTL, _ := s.ExpireTime("k"); hasTTL {
		t.Fatal("GetEx persist left a TTL")
	}

	// absolute deadline in the future
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExSetAt, At: time.Unix(2000, 0)}); !ok {
		t.Fatal("GetEx set-at not ok")
	}
	if at, _, _ := s.ExpireTime("k"); at.Unix() != 2000 {
		t.Fatalf("GetEx set-at deadline = %d, want 2000", at.Unix())
	}

	// absolute deadline in the past: returns the value but deletes the key
	if v, ok, err := s.GetEx("k", GetExOp{Mode: GetExSetAt, At: time.Unix(500, 0)}); err != nil || !ok || v != "v" {
		t.Fatalf("GetEx past-at = %q ok=%v err=%v", v, ok, err)
	}
	if s.Exists("k") != 0 {
		t.Fatal("GetEx with a past deadline did not delete the key")
	}

	// a WRONGTYPE key errors and its TTL is left untouched
	s.push("lst", true, []string{"x"})
	s.Expire("lst", 100*time.Second)
	if _, _, err := s.GetEx("lst", GetExOp{Mode: GetExPersist}); err != ErrWrongType {
		t.Fatalf("GetEx wrong type = %v, want ErrWrongType", err)
	}
	if _, hasTTL, _ := s.ExpireTime("lst"); !hasTTL {
		t.Fatal("GetEx on a WRONGTYPE key cleared its TTL")
	}
}

func TestMSetNX(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	// all keys new: writes everything, returns true
	if !s.MSetNX([]string{"a", "b", "c"}, []string{"1", "2", "3"}) {
		t.Fatal("MSetNX to a clean keyspace should be true")
	}
	for k, want := range map[string]string{"a": "1", "b": "2", "c": "3"} {
		if v, _, _ := s.Get(k); v != want {
			t.Fatalf("MSetNX %s = %q, want %q", k, v, want)
		}
	}

	// any key already present: writes nothing, returns false
	if s.MSetNX([]string{"d", "a", "e"}, []string{"4", "9", "5"}) {
		t.Fatal("MSetNX with an existing key should be false")
	}
	if s.Exists("d") != 0 || s.Exists("e") != 0 {
		t.Fatal("MSetNX must not write any key when one already exists")
	}
	if v, _, _ := s.Get("a"); v != "1" {
		t.Fatalf("MSetNX must not overwrite the existing key, a = %q", v)
	}

	// a WRONGTYPE key still counts as existing and blocks the whole op
	s.push("lst", true, []string{"x"})
	if s.MSetNX([]string{"f", "lst"}, []string{"6", "7"}) {
		t.Fatal("MSetNX with an existing non-string key should be false")
	}
	if s.Exists("f") != 0 {
		t.Fatal("MSetNX wrote a key alongside an existing non-string key")
	}

	// an expired key does not count as existing
	s.Set("g", "old", SetOptions{})
	s.Expire("g", time.Second)
	cur = cur.Add(2 * time.Second)
	if !s.MSetNX([]string{"g"}, []string{"new"}) {
		t.Fatal("MSetNX over an expired key should be true")
	}
	if v, _, _ := s.Get("g"); v != "new" {
		t.Fatalf("MSetNX over an expired key = %q, want new", v)
	}

	// duplicate keys in one call: the last value wins
	if !s.MSetNX([]string{"h", "h"}, []string{"first", "second"}) {
		t.Fatal("MSetNX with duplicate new keys should be true")
	}
	if v, _, _ := s.Get("h"); v != "second" {
		t.Fatalf("MSetNX duplicate key = %q, want second (last wins)", v)
	}
}

func TestMSetNXConcurrent(t *testing.T) {
	s := New()
	// Two keys on different shards, so every MSetNX locks two shards. Goroutines
	// race to claim the pair in opposite key orders: a wrong lock order would
	// deadlock this test, and -race flags any unsafe shard-map access. Exactly
	// one goroutine wins (all-or-nothing) and both keys carry that winner's mark.
	kx, ky := "mx", "my"
	for i := 0; s.shardFor(kx) == s.shardFor(ky); i++ {
		ky = "my" + strconv.Itoa(i)
	}

	const g = 32
	var winners int64
	var wg sync.WaitGroup
	for i := 0; i < g; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mark := strconv.Itoa(id)
			keys := []string{kx, ky}
			if id%2 == 1 {
				keys = []string{ky, kx} // reversed order to stress lock ordering
			}
			if s.MSetNX(keys, []string{mark, mark}) {
				atomic.AddInt64(&winners, 1)
			}
		}(i)
	}
	wg.Wait()

	if winners != 1 {
		t.Fatalf("MSetNX winners = %d, want exactly 1 (all-or-nothing under races)", winners)
	}
	vx, _, _ := s.Get(kx)
	vy, _, _ := s.Get(ky)
	if vx == "" || vx != vy {
		t.Fatalf("MSetNX torn write: %s=%q %s=%q, want one winner's mark on both", kx, vx, ky, vy)
	}
}

func TestZIncrBy(t *testing.T) {
	s := New()

	// a new member on a missing key starts from 0
	if sc, err := s.ZIncrBy("z", "a", 5); err != nil || sc != 5 {
		t.Fatalf("ZIncrBy new = %v %v", sc, err)
	}
	if sc, err := s.ZIncrBy("z", "a", 2.5); err != nil || sc != 7.5 {
		t.Fatalf("ZIncrBy existing = %v %v", sc, err)
	}
	if sc, _ := s.ZIncrBy("z", "a", -10); sc != -2.5 {
		t.Fatalf("ZIncrBy negative = %v", sc)
	}

	// +inf plus -inf is NaN: error, and the score is left untouched
	s.ZAdd("z", []ZMember{{Member: "big", Score: math.Inf(1)}})
	if _, err := s.ZIncrBy("z", "big", math.Inf(-1)); err != ErrScoreNaN {
		t.Fatalf("ZIncrBy NaN result = %v, want ErrScoreNaN", err)
	}
	if sc, _, _ := s.ZScore("z", "big"); !math.IsInf(sc, 1) {
		t.Fatalf("ZIncrBy NaN modified the score = %v", sc)
	}

	// WRONGTYPE
	s.Set("str", "v", SetOptions{})
	if _, err := s.ZIncrBy("str", "a", 1); err != ErrWrongType {
		t.Fatalf("ZIncrBy wrong type = %v", err)
	}
}

func TestZRank(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZMember{{"a", 1}, {"b", 2}, {"c", 3}})

	for m, want := range map[string]int{"a": 0, "b": 1, "c": 2} {
		if r, found, err := s.ZRank("z", m); err != nil || !found || r != want {
			t.Fatalf("ZRank %s = %d found=%v err=%v, want %d", m, r, found, err, want)
		}
	}
	for m, want := range map[string]int{"c": 0, "b": 1, "a": 2} {
		if r, found, _ := s.ZRevRank("z", m); !found || r != want {
			t.Fatalf("ZRevRank %s = %d found=%v, want %d", m, r, found, want)
		}
	}

	// ties are broken lexicographically by member
	s.ZAdd("t", []ZMember{{"y", 5}, {"x", 5}})
	if r, _, _ := s.ZRank("t", "x"); r != 0 {
		t.Fatalf("ZRank tie x = %d, want 0", r)
	}
	if r, _, _ := s.ZRank("t", "y"); r != 1 {
		t.Fatalf("ZRank tie y = %d, want 1", r)
	}

	// missing member and missing key both report found=false
	if _, found, _ := s.ZRank("z", "nope"); found {
		t.Fatal("ZRank missing member should be found=false")
	}
	if _, found, _ := s.ZRevRank("nokey", "a"); found {
		t.Fatal("ZRevRank missing key should be found=false")
	}

	// WRONGTYPE
	s.Set("str", "v", SetOptions{})
	if _, _, err := s.ZRank("str", "a"); err != ErrWrongType {
		t.Fatalf("ZRank wrong type = %v", err)
	}
}

func zmemberNames(ms []ZMember) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Member
	}
	return out
}

func TestZRevRange(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZMember{{"a", 1}, {"b", 2}, {"c", 3}})

	// full reverse range: highest score first
	r, err := s.ZRevRange("z", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(zmemberNames(r), ","); got != "c,b,a" {
		t.Fatalf("ZRevRange 0 -1 = %q, want c,b,a", got)
	}
	// partial range
	r, _ = s.ZRevRange("z", 0, 1)
	if got := strings.Join(zmemberNames(r), ","); got != "c,b" {
		t.Fatalf("ZRevRange 0 1 = %q, want c,b", got)
	}
	// start past the end -> empty; start > stop -> empty; missing key -> empty
	if r, _ := s.ZRevRange("z", 5, 10); len(r) != 0 {
		t.Fatalf("ZRevRange out-of-range = %v", r)
	}
	if r, _ := s.ZRevRange("z", 2, 1); len(r) != 0 {
		t.Fatalf("ZRevRange start>stop = %v", r)
	}
	if r, _ := s.ZRevRange("nokey", 0, -1); len(r) != 0 {
		t.Fatalf("ZRevRange missing = %v", r)
	}

	// equal scores break ties by REVERSE lexicographic order (Redis): ascending
	// order is m,x,y, so descending must be y,x,m — not just the scores flipped.
	s.ZAdd("t", []ZMember{{"x", 5}, {"y", 5}, {"m", 5}})
	r, _ = s.ZRevRange("t", 0, -1)
	if got := strings.Join(zmemberNames(r), ","); got != "y,x,m" {
		t.Fatalf("ZRevRange tie order = %q, want y,x,m", got)
	}

	// WRONGTYPE
	s.Set("str", "v", SetOptions{})
	if _, err := s.ZRevRange("str", 0, -1); err != ErrWrongType {
		t.Fatalf("ZRevRange wrong type = %v", err)
	}
}

func TestZCount(t *testing.T) {
	s := New()
	s.ZAdd("z", []ZMember{{"a", 1}, {"b", 2}, {"c", 3}, {"d", 3}})
	negInf, posInf := math.Inf(-1), math.Inf(1)

	cases := []struct {
		label   string
		min     float64
		minExcl bool
		max     float64
		maxExcl bool
		want    int
	}{
		{"-inf..+inf", negInf, false, posInf, false, 4},
		{"[2,3]", 2, false, 3, false, 3}, // b, c, d
		{"(2,3]", 2, true, 3, false, 2},  // c, d
		{"[2,3)", 2, false, 3, true, 1},  // b
		{"(2,3)", 2, true, 3, true, 0},
		{"[5,10]", 5, false, 10, false, 0},
		{"min>max", 3, false, 1, false, 0},
	}
	for _, tc := range cases {
		if n, err := s.ZCount("z", tc.min, tc.minExcl, tc.max, tc.maxExcl); err != nil || n != tc.want {
			t.Fatalf("ZCount %s = %d (err %v), want %d", tc.label, n, err, tc.want)
		}
	}

	if n, _ := s.ZCount("nokey", negInf, false, posInf, false); n != 0 {
		t.Fatalf("ZCount missing key = %d", n)
	}
	s.Set("str", "v", SetOptions{})
	if _, err := s.ZCount("str", 0, false, 10, false); err != ErrWrongType {
		t.Fatalf("ZCount wrong type = %v", err)
	}
}

// sameSet reports whether got contains exactly the want members, in any order.
func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	m := make(map[string]bool, len(got))
	for _, g := range got {
		m[g] = true
	}
	for _, w := range want {
		if !m[w] {
			return false
		}
	}
	return true
}

func TestSetAlgebra(t *testing.T) {
	s := New()
	s.SAdd("a", "1", "2", "3")
	s.SAdd("b", "2", "3", "4")
	s.SAdd("c", "3", "4", "5")

	check := func(label string, got []string, err error, want ...string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: unexpected error %v", label, err)
		}
		if !sameSet(got, want...) {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}

	r, err := s.SInter([]string{"a", "b", "c"})
	check("SInter abc", r, err, "3")
	r, err = s.SInter([]string{"a", "b"})
	check("SInter ab", r, err, "2", "3")
	r, err = s.SInter([]string{"a", "nope"}) // missing key -> empty
	check("SInter missing", r, err)
	s.SAdd("disjoint", "9")
	r, err = s.SInter([]string{"a", "disjoint"}) // non-empty but no overlap -> empty
	check("SInter disjoint", r, err)
	r, err = s.SInter([]string{"a"}) // single key -> the set itself
	check("SInter single", r, err, "1", "2", "3")

	r, err = s.SUnion([]string{"a", "b"})
	check("SUnion ab", r, err, "1", "2", "3", "4")
	r, err = s.SUnion([]string{"a", "nope"})
	check("SUnion missing", r, err, "1", "2", "3")

	r, err = s.SDiff([]string{"a", "b"})
	check("SDiff ab", r, err, "1")
	r, err = s.SDiff([]string{"a", "b", "c"})
	check("SDiff abc", r, err, "1")
	r, err = s.SDiff([]string{"nope", "a"}) // missing first -> empty
	check("SDiff missing-first", r, err)
	r, err = s.SDiff([]string{"a", "nope"}) // missing other -> first unchanged
	check("SDiff missing-other", r, err, "1", "2", "3")

	// a WRONGTYPE key anywhere fails the whole op
	s.Set("str", "v", SetOptions{})
	if _, err := s.SInter([]string{"a", "str"}); err != ErrWrongType {
		t.Fatalf("SInter wrong type = %v", err)
	}
	if _, err := s.SUnion([]string{"str"}); err != ErrWrongType {
		t.Fatalf("SUnion wrong type = %v", err)
	}
	if _, err := s.SDiff([]string{"str", "a"}); err != ErrWrongType {
		t.Fatalf("SDiff wrong type = %v", err)
	}
	// A missing key never shields a wrong-typed key after it: every key is
	// type-checked before the result is decided, so the reply does not depend
	// on argument order and all three commands agree, matching Redis.
	if _, err := s.SInter([]string{"a", "nope", "str"}); err != ErrWrongType {
		t.Fatalf("SInter missing-before-wrongtype = %v, want WRONGTYPE", err)
	}
	if _, err := s.SInter([]string{"a", "str", "nope"}); err != ErrWrongType {
		t.Fatalf("SInter wrongtype-before-missing = %v, want WRONGTYPE", err)
	}
	if _, err := s.SUnion([]string{"a", "nope", "str"}); err != ErrWrongType {
		t.Fatalf("SUnion missing-before-wrongtype = %v, want WRONGTYPE", err)
	}
	if _, err := s.SDiff([]string{"a", "nope", "str"}); err != ErrWrongType {
		t.Fatalf("SDiff missing-before-wrongtype = %v, want WRONGTYPE", err)
	}
}

func hasDup(xs []string) bool {
	seen := make(map[string]bool, len(xs))
	for _, x := range xs {
		if seen[x] {
			return true
		}
		seen[x] = true
	}
	return false
}

func TestSetPopRandom(t *testing.T) {
	s := New()

	// missing key
	if r, err := s.SPop("nope", 1); err != nil || r != nil {
		t.Fatalf("SPop missing = %v %v", r, err)
	}
	if r, err := s.SRandMember("nope", 1); err != nil || r != nil {
		t.Fatalf("SRandMember missing = %v %v", r, err)
	}

	s.SAdd("s", "a", "b", "c", "d")

	// SRandMember never modifies the set and returns real members
	r, err := s.SRandMember("s", 2)
	if err != nil || len(r) != 2 || hasDup(r) {
		t.Fatalf("SRandMember 2 = %v %v", r, err)
	}
	for _, m := range r {
		if ok, _ := s.SIsMember("s", m); !ok {
			t.Fatalf("SRandMember returned a non-member %q", m)
		}
	}
	if n, _ := s.SCard("s"); n != 4 {
		t.Fatal("SRandMember modified the set")
	}
	// a positive count larger than the set returns all members, still distinct
	r, _ = s.SRandMember("s", 100)
	if len(r) != 4 || hasDup(r) {
		t.Fatalf("SRandMember over-count = %v, want 4 distinct", r)
	}
	// a negative count returns exactly -count members (repeats allowed)
	r, _ = s.SRandMember("s", -10)
	if len(r) != 10 {
		t.Fatalf("SRandMember -10 = %d members, want 10", len(r))
	}
	// count 0 is empty
	if r, _ := s.SRandMember("s", 0); len(r) != 0 {
		t.Fatalf("SRandMember 0 = %v", r)
	}
	// a huge negative count errors rather than allocating unbounded / overflowing
	if _, err := s.SRandMember("s", math.MinInt64); err != ErrSampleCount {
		t.Fatalf("SRandMember min-int64 = %v, want ErrSampleCount", err)
	}
	// but the cap fires only after the key/type check: a missing key is still
	// empty, and a wrong type is still WRONGTYPE, even with a huge count.
	if r, err := s.SRandMember("nope", math.MinInt64); err != nil || r != nil {
		t.Fatalf("SRandMember missing huge = %v %v, want nil nil", r, err)
	}
	s.Set("str", "v", SetOptions{})
	if _, err := s.SRandMember("str", math.MinInt64); err != ErrWrongType {
		t.Fatalf("SRandMember wrong-type huge = %v, want WRONGTYPE", err)
	}

	// SPop removes what it returns
	popped, err := s.SPop("s", 2)
	if err != nil || len(popped) != 2 || hasDup(popped) {
		t.Fatalf("SPop 2 = %v %v", popped, err)
	}
	for _, m := range popped {
		if ok, _ := s.SIsMember("s", m); ok {
			t.Fatalf("SPop did not remove %q", m)
		}
	}
	if n, _ := s.SCard("s"); n != 2 {
		t.Fatalf("SPop left %d members, want 2", n)
	}
	// SPop of the whole set deletes the key
	if popped, _ := s.SPop("s", 100); len(popped) != 2 {
		t.Fatalf("SPop all = %v", popped)
	}
	if s.Exists("s") != 0 {
		t.Fatal("SPop emptying the set should delete the key")
	}
	// SPop 0 removes nothing
	s.SAdd("s2", "x")
	if popped, _ := s.SPop("s2", 0); len(popped) != 0 {
		t.Fatalf("SPop 0 = %v", popped)
	}
	if n, _ := s.SCard("s2"); n != 1 {
		t.Fatal("SPop 0 modified the set")
	}

	// WRONGTYPE
	s.Set("str", "v", SetOptions{})
	if _, err := s.SPop("str", 1); err != ErrWrongType {
		t.Fatalf("SPop wrong type = %v", err)
	}
	if _, err := s.SRandMember("str", 1); err != ErrWrongType {
		t.Fatalf("SRandMember wrong type = %v", err)
	}
}

func TestSetIndexInvariant(t *testing.T) {
	// Random adds and removes against a plain map: after every step each
	// member's idx must point at its own slot, or a later swap-remove would
	// corrupt the set.
	rng := rand.New(rand.NewSource(1))
	st := newSet(0)
	ref := map[string]bool{}
	for step := 0; step < 5000; step++ {
		m := "m" + strconv.Itoa(rng.Intn(64))
		if rng.Intn(2) == 0 {
			if st.add(m) == ref[m] {
				t.Fatalf("step %d: add(%s) disagreed with the reference", step, m)
			}
			ref[m] = true
		} else {
			if st.remove(m) != ref[m] {
				t.Fatalf("step %d: remove(%s) disagreed with the reference", step, m)
			}
			delete(ref, m)
		}
		if st.len() != len(ref) || len(st.idx) != len(ref) {
			t.Fatalf("step %d: len %d / idx %d, want %d", step, st.len(), len(st.idx), len(ref))
		}
		for i, item := range st.items {
			if st.idx[item] != i || !ref[item] {
				t.Fatalf("step %d: items[%d]=%s has idx %d (in ref: %v)", step, i, item, st.idx[item], ref[item])
			}
		}
	}
}

func TestSetSamplingUniform(t *testing.T) {
	// Every pick indexes the member slice uniformly, so each of 10 members
	// should come up at its expected rate. The bounds sit 7+ standard
	// deviations out, so this cannot flake, yet any positional bias (like
	// taking the first key of a map range) lands far outside them.
	s := New()
	members := make([]string, 10)
	for i := range members {
		members[i] = "m" + strconv.Itoa(i)
	}
	s.SAdd("s", members...)
	within := func(label string, counts map[string]int, want, tol int) {
		t.Helper()
		if len(counts) != len(members) {
			t.Fatalf("%s: saw %d distinct members, want %d", label, len(counts), len(members))
		}
		for m, c := range counts {
			if c < want-tol || c > want+tol {
				t.Fatalf("%s: %s picked %d times, want %d±%d (%v)", label, m, c, want, tol, counts)
			}
		}
	}

	counts := map[string]int{}
	for i := 0; i < 100000; i++ {
		r, _ := s.SRandMember("s", 1)
		counts[r[0]]++
	}
	within("SRANDMEMBER", counts, 10000, 1000)

	counts = map[string]int{}
	for i := 0; i < 50000; i++ {
		r, _ := s.SPop("s", 1)
		counts[r[0]]++
		s.SAdd("s", r[0]) // put it back so the set stays at 10
	}
	within("SPOP", counts, 5000, 500)

	// both sampleDistinct strategies: random draws (2 of 10) and partial shuffle (8 of 10)
	for _, k := range []int64{2, 8} {
		counts = map[string]int{}
		for i := 0; i < 50000; i++ {
			r, _ := s.SRandMember("s", k)
			if int64(len(r)) != k || hasDup(r) {
				t.Fatalf("SRANDMEMBER %d = %v, want %d distinct", k, r, k)
			}
			for _, m := range r {
				counts[m]++
			}
		}
		want := int(50000 * k / 10)
		within("SRANDMEMBER "+strconv.FormatInt(k, 10), counts, want, want/10)
	}
	if n, _ := s.SCard("s"); n != 10 {
		t.Fatalf("set ended with %d members, want 10", n)
	}
}

// A single SPOP/SRANDMEMBER used to copy the whole set to pick one member.
func BenchmarkSPopLargeSet(b *testing.B) {
	s := New()
	members := make([]string, 1_000_000)
	for i := range members {
		members[i] = "m" + strconv.Itoa(i)
	}
	s.SAdd("big", members...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		popped, _ := s.SPop("big", 1)
		s.SAdd("big", popped[0]) // keep the set at 1M
	}
}

func BenchmarkSRandMemberLargeSet(b *testing.B) {
	s := New()
	members := make([]string, 1_000_000)
	for i := range members {
		members[i] = "m" + strconv.Itoa(i)
	}
	s.SAdd("big", members...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SRandMember("big", 1)
	}
}

func TestRename(t *testing.T) {
	s := New()

	if err := s.Rename("nope", "dst"); err != ErrNoSuchKey {
		t.Fatalf("Rename missing src = %v, want ErrNoSuchKey", err)
	}

	// a missing source errors even when src == dst: the miss must win over the
	// self-rename short-circuit
	if err := s.Rename("ghost", "ghost"); err != ErrNoSuchKey {
		t.Fatalf("Rename missing self = %v, want ErrNoSuchKey", err)
	}

	// basic move: value follows, source disappears
	s.Set("a", "1", SetOptions{})
	if err := s.Rename("a", "b"); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	if v, ok, _ := s.Get("b"); !ok || v != "1" {
		t.Fatalf("after Rename b = %q ok=%v", v, ok)
	}
	if s.Exists("a") != 0 {
		t.Fatal("source should be gone after Rename")
	}

	// the TTL travels with the key
	s.Set("t", "v", SetOptions{})
	s.Expire("t", 100*time.Second)
	if err := s.Rename("t", "t2"); err != nil {
		t.Fatalf("Rename with TTL = %v", err)
	}
	if d, hasTTL, ok := s.TTL("t2"); !ok || !hasTTL || d <= 0 {
		t.Fatalf("TTL did not travel: d=%v hasTTL=%v ok=%v", d, hasTTL, ok)
	}

	// renaming over an existing key replaces it wholesale: the destination's own
	// TTL is dropped along with its old value
	s.Set("src", "new", SetOptions{})
	s.Set("dst", "old", SetOptions{})
	s.Expire("dst", 100*time.Second)
	if err := s.Rename("src", "dst"); err != nil {
		t.Fatalf("Rename overwrite = %v", err)
	}
	if v, ok, _ := s.Get("dst"); !ok || v != "new" {
		t.Fatalf("overwrite dst = %q ok=%v", v, ok)
	}
	if _, hasTTL, _ := s.TTL("dst"); hasTTL {
		t.Fatal("dst kept its old TTL; the source (no TTL) should have replaced it")
	}

	// rename works regardless of value type and discards the destination's type
	s.push("lst", true, []string{"x"})
	s.Set("strkey", "s", SetOptions{})
	if err := s.Rename("lst", "strkey"); err != nil {
		t.Fatalf("Rename list over string = %v", err)
	}
	if s.Type("strkey") != "list" {
		t.Fatalf("dst type after rename = %q, want list", s.Type("strkey"))
	}

	// rename to itself is a successful no-op
	s.Set("self", "v", SetOptions{})
	if err := s.Rename("self", "self"); err != nil {
		t.Fatalf("Rename self = %v", err)
	}
	if v, ok, _ := s.Get("self"); !ok || v != "v" {
		t.Fatalf("self after no-op rename = %q ok=%v", v, ok)
	}

	// pathological key names: empty source key and a 1 MiB destination key
	long := strings.Repeat("k", 1<<20)
	s.Set("", "empty-key", SetOptions{})
	if err := s.Rename("", long); err != nil {
		t.Fatalf("Rename empty->long = %v", err)
	}
	if v, ok, _ := s.Get(long); !ok || v != "empty-key" {
		t.Fatalf("long key value = %q ok=%v", v, ok)
	}
}

func TestRenameNX(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if _, err := s.RenameNX("nope", "dst"); err != ErrNoSuchKey {
		t.Fatalf("RenameNX missing src = %v, want ErrNoSuchKey", err)
	}

	// a missing source errors even when src == dst
	if _, err := s.RenameNX("ghost", "ghost"); err != ErrNoSuchKey {
		t.Fatalf("RenameNX missing self = %v, want ErrNoSuchKey", err)
	}

	// destination free: renames, reports true, TTL travels
	s.Set("a", "1", SetOptions{})
	s.Expire("a", 100*time.Second)
	if ok, err := s.RenameNX("a", "b"); err != nil || !ok {
		t.Fatalf("RenameNX to free dst = %v, %v; want true, nil", ok, err)
	}
	if v, got, _ := s.Get("b"); !got || v != "1" {
		t.Fatalf("RenameNX moved value = %q got=%v", v, got)
	}
	if _, hasTTL, _ := s.TTL("b"); !hasTTL {
		t.Fatal("RenameNX did not carry the TTL over")
	}

	// destination exists: no move, reports false, both keys untouched
	s.Set("x", "xv", SetOptions{})
	s.Set("y", "yv", SetOptions{})
	if ok, err := s.RenameNX("x", "y"); err != nil || ok {
		t.Fatalf("RenameNX to existing dst = %v, %v; want false, nil", ok, err)
	}
	if v, _, _ := s.Get("x"); v != "xv" {
		t.Fatal("RenameNX with existing dst should leave src in place")
	}
	if v, _, _ := s.Get("y"); v != "yv" {
		t.Fatal("RenameNX with existing dst should leave dst unchanged")
	}

	// rename to itself: the destination "exists", so it reports false
	s.Set("self", "v", SetOptions{})
	if ok, err := s.RenameNX("self", "self"); err != nil || ok {
		t.Fatalf("RenameNX self = %v, %v; want false, nil", ok, err)
	}
	if v, got, _ := s.Get("self"); !got || v != "v" {
		t.Fatalf("self key damaged by RenameNX no-op = %q got=%v", v, got)
	}

	// an expired destination does not count as existing
	s.Set("src2", "sv", SetOptions{})
	s.Set("dead", "dv", SetOptions{})
	s.Expire("dead", time.Second)
	cur = cur.Add(2 * time.Second)
	if ok, err := s.RenameNX("src2", "dead"); err != nil || !ok {
		t.Fatalf("RenameNX over expired dst = %v, %v; want true, nil", ok, err)
	}
	if v, got, _ := s.Get("dead"); !got || v != "sv" {
		t.Fatalf("RenameNX over expired dst value = %q got=%v", v, got)
	}
}

func TestRenameConcurrent(t *testing.T) {
	s := New()
	// Two keys guaranteed to live on different shards, so every rename locks a
	// pair of shards. Goroutines rename in both directions at once: a wrong lock
	// order would deadlock this test, and -race would flag any unsafe access to
	// the shard maps.
	a, b := "ra", "rb"
	for i := 0; s.shardIndex(a) == s.shardIndex(b); i++ {
		b = "rb" + strconv.Itoa(i)
	}
	s.Set(a, "v", SetOptions{})

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				s.Rename(a, b) // a -> b (may be a no-op if already moved)
				s.Rename(b, a) // b -> a
				s.Exists(a)    // concurrent readers taking read locks
				s.Exists(b)
			}
		}()
	}
	wg.Wait()

	// the single key is never duplicated or lost: exactly one survives
	if n := s.Exists(a) + s.Exists(b); n != 1 {
		t.Fatalf("after concurrent renames, live keys among {a,b} = %d, want 1", n)
	}
}

func TestRandomKey(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	// empty keyspace
	if _, ok := s.RandomKey(); ok {
		t.Fatal("RandomKey on an empty keyspace should be ok=false")
	}

	// a single key is always the one returned
	s.Set("only", "v", SetOptions{})
	if k, ok := s.RandomKey(); !ok || k != "only" {
		t.Fatalf("RandomKey single = %q ok=%v", k, ok)
	}

	// across a large keyspace every result is a live member of the set; the
	// distribution is intentionally not asserted (RANDOMKEY is best-effort)
	want := map[string]bool{"only": true}
	for i := 0; i < 1000; i++ {
		k := "k" + strconv.Itoa(i)
		s.Set(k, "v", SetOptions{})
		want[k] = true
	}
	for i := 0; i < 200; i++ {
		k, ok := s.RandomKey()
		if !ok || !want[k] {
			t.Fatalf("RandomKey returned %q ok=%v, not a live key", k, ok)
		}
	}

	// expired keys are never returned
	s.Flush()
	s.Set("live", "v", SetOptions{})
	s.Set("dead", "v", SetOptions{})
	s.Expire("dead", time.Second)
	cur = cur.Add(2 * time.Second)
	for i := 0; i < 100; i++ {
		if k, ok := s.RandomKey(); !ok || k != "live" {
			t.Fatalf("RandomKey should only return the live key, got %q ok=%v", k, ok)
		}
	}

	// a keyspace whose keys have all expired reports empty again
	s.Flush()
	s.Set("g1", "v", SetOptions{})
	s.Set("g2", "v", SetOptions{})
	s.Expire("g1", time.Second)
	s.Expire("g2", time.Second)
	cur = cur.Add(2 * time.Second)
	if _, ok := s.RandomKey(); ok {
		t.Fatal("RandomKey with only expired keys should be ok=false")
	}
}

func TestVectorExpirySweep(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	s.VSet("cache", "temp", []float32{1, 0}, "m", 1060)
	s.VSet("cache", "keep", []float32{0, 1}, "m", 0)
	s.VSet("gone", "only", []float32{1, 1}, "m", 1030)

	// Expired items stop matching the moment their deadline passes, before
	// any sweep runs, so an expired entry cannot outrank a live one.
	cur = time.Unix(1061, 0)
	res, err := s.VSearch("cache", []float32{1, 0}, 1)
	if err != nil || len(res) != 1 || res[0].Item.ID != "keep" {
		t.Fatalf("VSearch after expiry = %v %v, want keep only", res, err)
	}

	// The sweeper reclaims expired items without anyone querying them.
	s.sweep()
	if n, _ := s.VCard("cache"); n != 1 {
		t.Fatalf("VCard after sweep = %d, want 1", n)
	}
	// A collection whose items all expired drops the key entirely.
	if s.Exists("gone") != 0 {
		t.Fatal("sweep left an empty vector key behind")
	}
	// Items with no expiry survive any amount of time.
	cur = time.Unix(1e9, 0)
	s.sweep()
	if n, _ := s.VCard("cache"); n != 1 {
		t.Fatalf("sweep removed the immortal item: VCard = %d", n)
	}
}

func TestVectorExpirySnapshot(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }
	s.VSet("v", "dead", []float32{1, 0}, "m", 1010)
	s.VSet("v", "live", []float32{0, 1}, "m", 2000)
	s.VSet("v", "forever", []float32{1, 1}, "m", 0)

	// A collection whose items are all dead at save time writes no record.
	s.VSet("allgone", "x", []float32{1, 0}, "m", 1005)

	// Items already expired at save time are not written out.
	cur = time.Unix(1020, 0)
	var vrec *Record
	recs := s.Export()
	for i := range recs {
		if recs[i].Key == "v" {
			vrec = &recs[i]
		}
		if recs[i].Key == "allgone" {
			t.Fatalf("an all-expired collection was exported: %+v", recs[i])
		}
	}
	if vrec == nil || len(vrec.Vectors) != 2 {
		t.Fatalf("exported vectors = %+v, want live and forever only", vrec)
	}

	// Deadlines survive the round-trip: a store loading the snapshot past
	// live's deadline skips it, and forever stays.
	s2 := New()
	cur2 := time.Unix(3000, 0)
	s2.now = func() time.Time { return cur2 }
	s2.Import(recs)
	if n, _ := s2.VCard("v"); n != 1 {
		t.Fatalf("VCard after import past a deadline = %d, want 1", n)
	}
	if res, _ := s2.VSearch("v", []float32{1, 1}, 10); len(res) != 1 || res[0].Item.ID != "forever" {
		t.Fatalf("VSearch after import = %v, want forever only", res)
	}

	// A record written before the expiry field existed decodes with
	// ExpireUnix 0 and must load as never expiring.
	s2.Import([]Record{{Key: "old", Type: "vector", VecDim: 2,
		Vectors: []VecRecord{{ID: "x", Vec: []float32{1, 0}, Meta: "m"}}}})
	s2.sweep()
	if n, _ := s2.VCard("old"); n != 1 {
		t.Fatalf("pre-upgrade record did not load as immortal: VCard = %d", n)
	}

	// An all-expired collection resurrects no key at load.
	s2.Import([]Record{{Key: "husk", Type: "vector", VecDim: 2,
		Vectors: []VecRecord{{ID: "x", Vec: []float32{1, 0}, Meta: "m", ExpireUnix: 5}}}})
	if s2.Exists("husk") != 0 {
		t.Fatal("an all-expired collection loaded as an empty key")
	}
}

// TestBitOpConcurrent runs BITOP over overlapping keys in every order while
// SETBIT writes to them. Shards must be locked in a fixed order, or two BITOPs
// with swapped keys deadlock; run with -race to also catch unlocked access.
func TestBitOpConcurrent(t *testing.T) {
	s := New()
	keys := []string{"a", "b", "c", "d", "e", "f"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 2000; i++ {
				k := keys[r.Intn(len(keys))]
				if r.Intn(2) == 0 {
					if _, err := s.SetBit(k, int64(r.Intn(256)), r.Intn(2) == 0); err != nil {
						t.Error(err)
						return
					}
					continue
				}
				srcs := []string{keys[r.Intn(len(keys))], keys[r.Intn(len(keys))]}
				if _, err := s.BitOp(BitOpKind(r.Intn(3)), k, srcs); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("BITOP deadlocked")
	}
}
