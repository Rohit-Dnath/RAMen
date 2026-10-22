package server

import (
	"strconv"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

func (c *conn) cmdSAdd(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("sadd")
	}
	n, err := c.s.store.SAdd(args[1], args[2:]...)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdSRem(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("srem")
	}
	n, err := c.s.store.SRem(args[1], args[2:]...)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdSMembers(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("smembers")
	}
	ms, err := c.s.store.SMembers(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(ms)
}

func (c *conn) cmdSIsMember(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("sismember")
	}
	ok, err := c.s.store.SIsMember(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdSCard(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("scard")
	}
	n, err := c.s.store.SCard(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdSInter(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("sinter")
	}
	ms, err := c.s.store.SInter(args[1:])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(ms)
}

func (c *conn) cmdSUnion(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("sunion")
	}
	ms, err := c.s.store.SUnion(args[1:])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(ms)
}

func (c *conn) cmdSDiff(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("sdiff")
	}
	ms, err := c.s.store.SDiff(args[1:])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(ms)
}

// cmdSPop implements SPOP key [count]. Without a count it removes and returns a
// single random member as a bulk string (nil when the set is missing/empty);
// with a count it returns an array of up to count removed members.
func (c *conn) cmdSPop(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return c.wrongArgs("spop")
	}
	if len(args) == 2 {
		res, err := c.s.store.SPop(args[1], 1)
		if err != nil {
			return c.storeErr(err)
		}
		if len(res) == 0 {
			return c.writeNull()
		}
		return c.writeBulk(res[0])
	}
	count, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	if count < 0 {
		return c.writeError("ERR value is out of range, must be positive")
	}
	res, err := c.s.store.SPop(args[1], count)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(res)
}

// cmdSRandMember implements SRANDMEMBER key [count]. Without a count it returns a
// single random member as a bulk string (nil when missing); with a positive
// count up to that many distinct members, with a negative count exactly -count
// members allowing repeats. It never modifies the set.
func (c *conn) cmdSRandMember(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return c.wrongArgs("srandmember")
	}
	if len(args) == 2 {
		res, err := c.s.store.SRandMember(args[1], 1)
		if err != nil {
			return c.storeErr(err)
		}
		if len(res) == 0 {
			return c.writeNull()
		}
		return c.writeBulk(res[0])
	}
	count, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	res, err := c.s.store.SRandMember(args[1], count)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(res)
}
