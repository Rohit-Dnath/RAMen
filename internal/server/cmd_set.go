package server

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
