package server

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

// maxStringSize caps a string value at 512MB, matching Redis' proto-max-bulk-len.
const maxStringSize = 512 * 1024 * 1024

func (c *conn) cmdGet(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("get")
	}
	v, ok, err := c.s.store.Get(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

// cmdSet implements SET key value [EX s | PX ms] [NX | XX].
func (c *conn) cmdSet(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("set")
	}
	key, val := args[1], args[2]
	var opts store.SetOptions
	for i := 3; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "EX", "PX":
			if i+1 >= len(args) {
				return c.writeError("ERR syntax error")
			}
			n, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil {
				return c.writeError("ERR value is not an integer or out of range")
			}
			unit := time.Second
			if strings.ToUpper(args[i]) == "PX" {
				unit = time.Millisecond
			}
			opts.TTL = time.Duration(n) * unit
			opts.HasEx = true
			i++
		case "NX":
			opts.NX = true
		case "XX":
			opts.XX = true
		default:
			return c.writeError("ERR syntax error")
		}
	}
	if c.s.store.Set(key, val, opts) {
		return c.writeSimple("OK")
	}
	return c.writeNull() // NX/XX condition not met
}

// cmdSetNX sets key to value only if it does not already exist, returning 1 on
// write and 0 when the key is already present (of any type).
func (c *conn) cmdSetNX(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("setnx")
	}
	ok := c.s.store.Set(args[1], args[2], store.SetOptions{NX: true})
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdSetEx(args []string) error  { return c.setEx(args, "setex", time.Second) }
func (c *conn) cmdPSetEx(args []string) error { return c.setEx(args, "psetex", time.Millisecond) }

// setEx implements SETEX/PSETEX: an atomic set with a positive TTL, in seconds
// or milliseconds depending on unit. Redis rejects a non-positive TTL.
func (c *conn) setEx(args []string, name string, unit time.Duration) error {
	if len(args) != 4 {
		return c.wrongArgs(name)
	}
	n, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	// Reject a non-positive TTL, and one so large that n*unit would overflow
	// time.Duration and wrap into the past (which would silently drop the key).
	if n <= 0 || n > math.MaxInt64/int64(unit) {
		return c.writeError("ERR invalid expire time in '" + name + "' command")
	}
	c.s.store.Set(args[1], args[3], store.SetOptions{TTL: time.Duration(n) * unit, HasEx: true})
	return c.writeSimple("OK")
}

func (c *conn) cmdGetSet(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("getset")
	}
	old, had, err := c.s.store.GetSet(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	if !had {
		return c.writeNull()
	}
	return c.writeBulk(old)
}

func (c *conn) cmdGetDel(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("getdel")
	}
	v, ok, err := c.s.store.GetDel(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

// cmdGetEx implements GETEX key [EX s | PX ms | EXAT ts | PXAT ts | PERSIST].
// With no option it behaves like GET, leaving the TTL untouched.
func (c *conn) cmdGetEx(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("getex")
	}
	op := store.GetExOp{Mode: store.GetExNoChange}
	if len(args) > 2 {
		switch opt := strings.ToUpper(args[2]); opt {
		case "PERSIST":
			if len(args) != 3 {
				return c.writeError("ERR syntax error")
			}
			op.Mode = store.GetExPersist
		case "EX", "PX", "EXAT", "PXAT":
			if len(args) != 4 {
				return c.writeError("ERR syntax error")
			}
			n, err := strconv.ParseInt(args[3], 10, 64)
			if err != nil {
				return c.writeError(store.ErrNotInteger.Error())
			}
			switch opt {
			case "EX", "PX":
				unit := time.Second
				if opt == "PX" {
					unit = time.Millisecond
				}
				// reject a non-positive TTL, and one so large it would overflow
				// time.Duration and wrap into the past
				if n <= 0 || n > math.MaxInt64/int64(unit) {
					return c.writeError("ERR invalid expire time in 'getex' command")
				}
				op.Mode = store.GetExSetTTL
				op.TTL = time.Duration(n) * unit
			case "EXAT":
				// Redis rejects a non-positive absolute expire up front and
				// leaves the key untouched; a positive-but-past one (e.g. EXAT 1)
				// still deletes it.
				if n <= 0 || n > maxExpireSeconds {
					return c.writeError("ERR invalid expire time in 'getex' command")
				}
				op.Mode = store.GetExSetAt
				op.At = time.Unix(n, 0)
			case "PXAT":
				if n <= 0 {
					return c.writeError("ERR invalid expire time in 'getex' command")
				}
				op.Mode = store.GetExSetAt
				op.At = time.UnixMilli(n)
			}
		default:
			return c.writeError("ERR syntax error")
		}
	}
	v, ok, err := c.s.store.GetEx(args[1], op)
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

func (c *conn) cmdAppend(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("append")
	}
	n, err := c.s.store.Append(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdIncr(args []string) error { return c.incrBy(args, "incr", 1) }
func (c *conn) cmdDecr(args []string) error { return c.incrBy(args, "decr", -1) }

func (c *conn) incrBy(args []string, name string, sign int64) error {
	if len(args) != 2 {
		return c.wrongArgs(name)
	}
	n, err := c.s.store.IncrBy(args[1], sign)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(n)
}

func (c *conn) cmdIncrBy(args []string) error { return c.incrByN(args, "incrby", 1) }
func (c *conn) cmdDecrBy(args []string) error { return c.incrByN(args, "decrby", -1) }

func (c *conn) incrByN(args []string, name string, sign int64) error {
	if len(args) != 3 {
		return c.wrongArgs(name)
	}
	delta, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	n, err := c.s.store.IncrBy(args[1], sign*delta)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(n)
}

func (c *conn) cmdIncrByFloat(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("incrbyfloat")
	}
	delta, err := strconv.ParseFloat(args[2], 64)
	if err != nil || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return c.writeError(store.ErrNotFloat.Error())
	}
	v, err := c.s.store.IncrByFloat(args[1], delta)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeBulk(v)
}

func (c *conn) cmdMGet(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("mget")
	}
	return c.writeArray(len(args)-1, func(w respWriter) error {
		for _, k := range args[1:] {
			v, ok, err := c.s.store.Get(k)
			if err != nil || !ok {
				if err := w.WriteNull(); err != nil {
					return err
				}
				continue
			}
			if err := w.WriteBulkString(v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *conn) cmdMSet(args []string) error {
	if len(args) < 3 || len(args)%2 != 1 {
		return c.wrongArgs("mset")
	}
	for i := 1; i+1 < len(args); i += 2 {
		c.s.store.Set(args[i], args[i+1], store.SetOptions{})
	}
	return c.writeSimple("OK")
}

// cmdMSetNX sets all key/value pairs only if none of the keys already exist,
// returning 1 on write and 0 (writing nothing) if any key is already present.
func (c *conn) cmdMSetNX(args []string) error {
	if len(args) < 3 || len(args)%2 != 1 {
		return c.wrongArgs("msetnx")
	}
	n := (len(args) - 1) / 2
	keys := make([]string, n)
	vals := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = args[1+2*i]
		vals[i] = args[2+2*i]
	}
	return c.writeInt(boolToInt(c.s.store.MSetNX(keys, vals)))
}

func (c *conn) cmdStrLen(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("strlen")
	}
	v, ok, err := c.s.store.Get(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeInt(0)
	}
	return c.writeInt(int64(len(v)))
}

func (c *conn) cmdGetRange(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("getrange")
	}
	start, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	end, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	v, err := c.s.store.GetRange(args[1], start, end)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeBulk(v)
}

func (c *conn) cmdSetRange(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("setrange")
	}
	offset, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	if offset < 0 {
		return c.writeError("ERR offset is out of range")
	}
	// Bound the size before allocating (overflow-safe), like Redis' proto-max-bulk-len.
	if offset > maxStringSize-int64(len(args[3])) {
		return c.writeError("ERR string exceeds maximum allowed size (proto-max-bulk-len)")
	}
	n, err := c.s.store.SetRange(args[1], int(offset), args[3])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

// parseStrictInt parses s as an int64 the way Redis does: no sign prefix,
// leading zeros or spaces, so "+3" and "007" are rejected.
func parseStrictInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != s {
		return 0, false
	}
	return n, true
}

// parseBitOffset parses a SETBIT/GETBIT offset, which must address a bit
// inside a maxStringSize string.
func parseBitOffset(s string) (int64, bool) {
	n, ok := parseStrictInt(s)
	if !ok || n < 0 || n>>3 >= maxStringSize {
		return 0, false
	}
	return n, true
}

func (c *conn) cmdSetBit(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("setbit")
	}
	offset, ok := parseBitOffset(args[2])
	if !ok {
		return c.writeError("ERR bit offset is not an integer or out of range")
	}
	if args[3] != "0" && args[3] != "1" {
		return c.writeError("ERR bit is not an integer or out of range")
	}
	old, err := c.s.store.SetBit(args[1], offset, args[3] == "1")
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(old))
}

func (c *conn) cmdGetBit(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("getbit")
	}
	offset, ok := parseBitOffset(args[2])
	if !ok {
		return c.writeError("ERR bit offset is not an integer or out of range")
	}
	bit, err := c.s.store.GetBit(args[1], offset)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(bit))
}

func (c *conn) cmdBitCount(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("bitcount")
	}
	start, end, bitUnit := int64(0), int64(-1), false
	switch len(args) {
	case 2:
	case 4, 5:
		var ok1, ok2 bool
		start, ok1 = parseStrictInt(args[2])
		end, ok2 = parseStrictInt(args[3])
		if !ok1 || !ok2 {
			return c.writeError(store.ErrNotInteger.Error())
		}
		if len(args) == 5 {
			switch strings.ToUpper(args[4]) {
			case "BYTE":
			case "BIT":
				bitUnit = true
			default:
				return c.writeError("ERR syntax error")
			}
		}
	default:
		return c.writeError("ERR syntax error")
	}
	n, err := c.s.store.BitCount(args[1], start, end, bitUnit)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(n)
}

func (c *conn) cmdBitOp(args []string) error {
	if len(args) < 4 {
		return c.wrongArgs("bitop")
	}
	var op store.BitOpKind
	switch strings.ToUpper(args[1]) {
	case "AND":
		op = store.BitAnd
	case "OR":
		op = store.BitOr
	case "XOR":
		op = store.BitXor
	case "NOT":
		op = store.BitNot
		if len(args) != 4 {
			return c.writeError("ERR BITOP NOT must be called with a single source key.")
		}
	default:
		return c.writeError("ERR syntax error")
	}
	n, err := c.s.store.BitOp(op, args[2], args[3:])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}
