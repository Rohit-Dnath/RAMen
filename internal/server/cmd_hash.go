package server

func (c *conn) cmdHSet(args []string) error {
	// HSET key field value [field value ...]
	if len(args) < 4 || len(args)%2 != 0 {
		return c.wrongArgs("hset")
	}
	pairs := make(map[string]string, (len(args)-2)/2)
	for i := 2; i+1 < len(args); i += 2 {
		pairs[args[i]] = args[i+1]
	}
	n, err := c.s.store.HSet(args[1], pairs)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdHGet(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("hget")
	}
	v, ok, err := c.s.store.HGet(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

func (c *conn) cmdHDel(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("hdel")
	}
	n, err := c.s.store.HDel(args[1], args[2:]...)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdHGetAll(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("hgetall")
	}
	flat, err := c.s.store.HGetAll(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(flat)
}

func (c *conn) cmdHKeys(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("hkeys")
	}
	ks, err := c.s.store.HKeys(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(ks)
}

func (c *conn) cmdHVals(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("hvals")
	}
	vs, err := c.s.store.HVals(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(vs)
}

func (c *conn) cmdHLen(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("hlen")
	}
	n, err := c.s.store.HLen(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdHExists(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("hexists")
	}
	_, ok, err := c.s.store.HGet(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdHMGet(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("hmget")
	}
	return c.writeArray(len(args)-2, func(w respWriter) error {
		for _, f := range args[2:] {
			v, ok, err := c.s.store.HGet(args[1], f)
			if err != nil || !ok {
				if e := w.WriteNull(); e != nil {
					return e
				}
				continue
			}
			if e := w.WriteBulkString(v); e != nil {
				return e
			}
		}
		return nil
	})
}
