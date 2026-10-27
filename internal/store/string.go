package store

import (
	"errors"
	"math"
	"math/bits"
	"strconv"
	"time"
)

// ErrWrongType mirrors Redis' WRONGTYPE condition: an operation was attempted
// against a key holding a different data type.
var ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// ErrNotInteger is returned when a string value cannot be parsed as an int64
// for INCR/DECR-style operations.
var ErrNotInteger = errors.New("ERR value is not an integer or out of range")

// ErrIntegerOverflow is returned when an increment would overflow int64.
var ErrIntegerOverflow = errors.New("ERR increment or decrement would overflow")

// ErrNotFloat is returned when a value cannot be parsed as a finite float64.
var ErrNotFloat = errors.New("ERR value is not a valid float")

// ErrFloatOverflow is returned when a float increment would produce NaN or Infinity.
var ErrFloatOverflow = errors.New("ERR increment would produce NaN or Infinity")

// ErrHashNotInteger mirrors Redis' HINCRBY error for a field whose value is not an integer.
var ErrHashNotInteger = errors.New("ERR hash value is not an integer")

// ErrHashNotFloat mirrors Redis' HINCRBYFLOAT error for a field whose value is not a float.
var ErrHashNotFloat = errors.New("ERR hash value is not a float")

// asString returns the string value of e, or ErrWrongType.
func asString(e *entry) (string, error) {
	s, ok := e.val.(string)
	if !ok {
		return "", ErrWrongType
	}
	return s, nil
}

// Get returns the string at key. ok is false if the key is missing.
func (s *Store) Get(key string) (val string, ok bool, err error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", false, nil
	}
	str, err := asString(e)
	if err != nil {
		return "", false, err
	}
	return str, true, nil
}

// SetOptions controls SET behaviour (EX/PX/NX/XX flags).
type SetOptions struct {
	TTL   time.Duration // 0 means no expiry
	HasEx bool          // an EX/PX flag was supplied
	NX    bool          // only set if the key does not exist
	XX    bool          // only set if the key already exists
}

// Set assigns a string value to key, honouring the supplied options. It
// reports whether the write happened (NX/XX can suppress it).
func (s *Store) Set(key, val string, opts SetOptions) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	_, exists := sh.getLive(key, s.now())
	if opts.NX && exists {
		return false
	}
	if opts.XX && !exists {
		return false
	}
	ne := &entry{val: val}
	if opts.HasEx {
		ne.expireAt = s.now().Add(opts.TTL)
	}
	sh.m[key] = ne
	return true
}

// MSetNX sets every key to its paired value, but only if NONE of the keys
// already exist. It is all-or-nothing (nothing is written when any key is
// present) and reports whether the write happened. keys and vals must be the
// same length.
func (s *Store) MSetNX(keys, vals []string) bool {
	// Lock every shard that hosts one of the keys, once each, in array-index
	// order so concurrent multi-key writers acquire the shards in the same order
	// and cannot deadlock.
	shs := make([]*shard, len(keys))
	involved := make(map[*shard]bool, len(keys))
	for i, k := range keys {
		shs[i] = s.shardFor(k)
		involved[shs[i]] = true
	}
	for _, sh := range s.shards {
		if involved[sh] {
			sh.mu.Lock()
		}
	}
	defer func() {
		for _, sh := range s.shards {
			if involved[sh] {
				sh.mu.Unlock()
			}
		}
	}()

	now := s.now()
	for i, k := range keys {
		if _, exists := shs[i].peekLive(k, now); exists {
			return false
		}
	}
	for i, k := range keys {
		shs[i].m[k] = &entry{val: vals[i]}
	}
	return true
}

// GetSet sets key to val and returns the previous string value.
func (s *Store) GetSet(key, val string) (old string, hadOld bool, err error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if found {
		if old, err = asString(e); err != nil {
			return "", false, err
		}
		hadOld = true
	}
	sh.m[key] = &entry{val: val}
	return old, hadOld, nil
}

// GetDel returns the string at key and deletes it, atomically. ok is false when
// the key is missing (nothing is deleted); a WRONGTYPE key is left in place.
func (s *Store) GetDel(key string) (val string, ok bool, err error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", false, nil
	}
	str, err := asString(e)
	if err != nil {
		return "", false, err
	}
	delete(sh.m, key)
	return str, true, nil
}

// GetExMode selects the TTL change GETEX applies after reading a key.
type GetExMode int

const (
	GetExNoChange GetExMode = iota // leave the TTL untouched (plain GET)
	GetExSetTTL                    // set a relative TTL: expireAt = now + TTL
	GetExSetAt                     // set an absolute deadline (delete if already past)
	GetExPersist                   // remove any TTL
)

// GetExOp describes GETEX's optional TTL change.
type GetExOp struct {
	Mode GetExMode
	TTL  time.Duration // used when Mode == GetExSetTTL
	At   time.Time     // used when Mode == GetExSetAt
}

// GetEx returns the string at key and optionally adjusts its TTL per op: no
// change (like GET), a relative TTL, an absolute deadline (a past one deletes
// the key while still returning the value, matching Redis), or PERSIST. ok is
// false when the key is missing.
func (s *Store) GetEx(key string, op GetExOp) (val string, ok bool, err error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", false, nil
	}
	str, err := asString(e)
	if err != nil {
		return "", false, err
	}
	switch op.Mode {
	case GetExSetTTL:
		e.expireAt = s.now().Add(op.TTL)
	case GetExSetAt:
		if !op.At.After(s.now()) {
			delete(sh.m, key)
			return str, true, nil
		}
		e.expireAt = op.At
	case GetExPersist:
		e.expireAt = time.Time{}
	}
	return str, true, nil
}

// Append concatenates val to the string at key (creating it if absent) and
// returns the new length.
func (s *Store) Append(key, val string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		sh.m[key] = &entry{val: val}
		return len(val), nil
	}
	cur, err := asString(e)
	if err != nil {
		return 0, err
	}
	e.val = cur + val
	return len(e.val.(string)), nil
}

// IncrBy adds delta to the integer string at key (treating a missing key as 0)
// and returns the new value.
func (s *Store) IncrBy(key string, delta int64) (int64, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var cur int64
	if found {
		str, err := asString(e)
		if err != nil {
			return 0, err
		}
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0, ErrNotInteger
		}
		cur = n
	}
	cur += delta
	if found {
		e.val = strconv.FormatInt(cur, 10)
	} else {
		sh.m[key] = &entry{val: strconv.FormatInt(cur, 10)}
	}
	return cur, nil
}

// IncrByFloat adds delta to the float string at key (treating a missing key as
// 0) and returns the new value formatted the way Redis does.
func (s *Store) IncrByFloat(key string, delta float64) (string, error) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return "", ErrNotFloat
	}
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var cur float64
	if found {
		str, err := asString(e)
		if err != nil {
			return "", err
		}
		n, err := strconv.ParseFloat(str, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return "", ErrNotFloat
		}
		cur = n
	}
	cur += delta
	if math.IsNaN(cur) || math.IsInf(cur, 0) {
		return "", ErrFloatOverflow
	}
	// normalize negative zero so we return "0", not "-0", like Redis
	if cur == 0 {
		cur = 0
	}
	out := strconv.FormatFloat(cur, 'f', -1, 64)
	if found {
		e.val = out
	} else {
		sh.m[key] = &entry{val: out}
	}
	return out, nil
}

// GetRange returns the substring between start and end (inclusive), with Redis-style
// negative offsets and clamping. A missing key yields an empty string.
func (s *Store) GetRange(key string, start, end int64) (string, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", nil
	}
	str, err := asString(e)
	if err != nil {
		return "", err
	}
	n := int64(len(str))
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= n {
		end = n - 1
	}
	if start > end {
		return "", nil
	}
	return str[start : end+1], nil
}

// SetRange overwrites from offset with val, zero-padding past the current end.
// It returns the length of the string after the write.
func (s *Store) SetRange(key string, offset int, val string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	var cur string
	e, found := sh.getLive(key, s.now())
	if found {
		str, err := asString(e)
		if err != nil {
			return 0, err
		}
		cur = str
	}
	if val == "" {
		return len(cur), nil
	}
	size := offset + len(val)
	if len(cur) > size {
		size = len(cur)
	}
	buf := make([]byte, size)
	copy(buf, cur)
	copy(buf[offset:], val)
	if found {
		e.val = string(buf)
	} else {
		sh.m[key] = &entry{val: string(buf)}
	}
	return size, nil
}

// BitOpKind selects the operation BitOp applies.
type BitOpKind int

const (
	BitAnd BitOpKind = iota
	BitOr
	BitXor
	BitNot
)

// SetBit sets the bit at offset to on and returns the bit's previous value.
// Bit 0 is the most significant bit of the first byte, as in Redis. The
// string is zero-padded when offset lies past its end, and any TTL is kept.
func (s *Store) SetBit(key string, offset int64, on bool) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	var cur string
	e, found := sh.getLive(key, s.now())
	if found {
		str, err := asString(e)
		if err != nil {
			return 0, err
		}
		cur = str
	}
	idx := int(offset >> 3)
	mask := byte(0x80) >> (offset & 7)
	size := max(len(cur), idx+1)
	buf := make([]byte, size)
	copy(buf, cur)
	old := 0
	if buf[idx]&mask != 0 {
		old = 1
	}
	if on {
		buf[idx] |= mask
	} else {
		buf[idx] &^= mask
	}
	if found {
		e.val = string(buf)
	} else {
		sh.m[key] = &entry{val: string(buf)}
	}
	return old, nil
}

// GetBit returns the bit at offset. A missing key, or an offset past the end
// of the string, reads as 0.
func (s *Store) GetBit(key string, offset int64) (int, error) {
	str, _, err := s.Get(key)
	if err != nil {
		return 0, err
	}
	idx := offset >> 3
	if idx >= int64(len(str)) {
		return 0, nil
	}
	if str[idx]&(byte(0x80)>>(offset&7)) != 0 {
		return 1, nil
	}
	return 0, nil
}

// BitCount counts the set bits between start and end (inclusive), with
// GETRANGE-style negative offsets and clamping. The range is in bytes, or in
// bits when bitUnit is set. Pass 0, -1 to count the whole string.
func (s *Store) BitCount(key string, start, end int64, bitUnit bool) (int64, error) {
	str, _, err := s.Get(key)
	if err != nil {
		return 0, err
	}
	n := int64(len(str))
	if bitUnit {
		n *= 8
	}
	// Redis checks this before clamping: e.g. -1 -3 is an empty range even
	// on a short string, where both would otherwise clamp to 0.
	if start < 0 && end < 0 && start > end {
		return 0, nil
	}
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end >= n {
		end = n - 1
	}
	if start > end {
		return 0, nil
	}
	if !bitUnit {
		var count int64
		for i := start; i <= end; i++ {
			count += int64(bits.OnesCount8(str[i]))
		}
		return count, nil
	}
	// Bit range: mask off the bits outside [start, end] in the first and
	// last bytes, count whole bytes in between.
	first, last := start>>3, end>>3
	var count int64
	for i := first; i <= last; i++ {
		b := str[i]
		if i == first {
			b &= 0xff >> (start & 7)
		}
		if i == last {
			b &= 0xff << (7 - end&7)
		}
		count += int64(bits.OnesCount8(b))
	}
	return count, nil
}

// BitOp combines the strings at keys bit by bit and stores the result at
// dest, returning its length. Shorter (or missing) sources are treated as
// zero-padded to the longest. dest is overwritten whatever its type and loses
// any TTL; when the result is empty dest is deleted instead. BitNot takes
// exactly one key; the caller enforces that.
func (s *Store) BitOp(op BitOpKind, dest string, keys []string) (int, error) {
	// Lock every involved shard once, in shard order, like MSetNX, so the
	// sources and dest are read and written as one atomic step.
	destSh := s.shardFor(dest)
	involved := map[*shard]bool{destSh: true}
	for _, k := range keys {
		involved[s.shardFor(k)] = true
	}
	for _, sh := range s.shards {
		if involved[sh] {
			sh.mu.Lock()
		}
	}
	defer func() {
		for _, sh := range s.shards {
			if involved[sh] {
				sh.mu.Unlock()
			}
		}
	}()

	now := s.now()
	srcs := make([]string, len(keys))
	size := 0
	for i, k := range keys {
		e, found := s.shardFor(k).getLive(k, now)
		if !found {
			continue
		}
		str, err := asString(e)
		if err != nil {
			return 0, err
		}
		srcs[i] = str
		size = max(size, len(str))
	}
	if size == 0 {
		delete(destSh.m, dest)
		return 0, nil
	}

	out := make([]byte, size)
	copy(out, srcs[0])
	if op == BitNot {
		for i := range out {
			out[i] = ^out[i]
		}
	}
	for _, src := range srcs[1:] {
		for i := range out {
			var b byte // past the end of src reads as zero
			if i < len(src) {
				b = src[i]
			}
			switch op {
			case BitAnd:
				out[i] &= b
			case BitOr:
				out[i] |= b
			case BitXor:
				out[i] ^= b
			}
		}
	}
	destSh.m[dest] = &entry{val: string(out)}
	return size, nil
}
