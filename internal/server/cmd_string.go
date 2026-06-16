package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

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
