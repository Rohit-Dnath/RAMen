package server

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

// registerCommands wires every supported command name to its handler. Names
// are stored upper-case; dispatch upper-cases the incoming command first.
func (s *Server) registerCommands() {
	s.dispatch = map[string]handler{
		// connection / server
		"PING":     (*conn).cmdPing,
		"ECHO":     (*conn).cmdEcho,
		"QUIT":     (*conn).cmdQuit,
		"SELECT":   (*conn).cmdSelect,
		"AUTH":     (*conn).cmdAuth,
		"COMMAND":  (*conn).cmdCommand,
		"INFO":     (*conn).cmdInfo,
		"DBSIZE":   (*conn).cmdDBSize,
		"FLUSHDB":  (*conn).cmdFlush,
		"FLUSHALL": (*conn).cmdFlush,
		"SAVE":     (*conn).cmdSave,
		"BGSAVE":   (*conn).cmdSave,

		// generic keyspace
		"DEL":             (*conn).cmdDel,
		"EXISTS":          (*conn).cmdExists,
		"EXPIRE":          (*conn).cmdExpire,
		"PEXPIRE":         (*conn).cmdPExpire,
		"EXPIREAT":        (*conn).cmdExpireAt,
		"PEXPIREAT":       (*conn).cmdPExpireAt,
		"TTL":             (*conn).cmdTTL,
		"PTTL":            (*conn).cmdPTTL,
		"EXPIRETIME":      (*conn).cmdExpireTime,
		"PERSIST":         (*conn).cmdPersist,
		"KEYS":            (*conn).cmdKeys,
		"TYPE":            (*conn).cmdType,
		"OBJECT ENCODING": (*conn).cmdObjectEncoding,

		// strings
		"GET":         (*conn).cmdGet,
		"SET":         (*conn).cmdSet,
		"SETNX":       (*conn).cmdSetNX,
		"SETEX":       (*conn).cmdSetEx,
		"PSETEX":      (*conn).cmdPSetEx,
		"GETSET":      (*conn).cmdGetSet,
		"APPEND":      (*conn).cmdAppend,
		"STRLEN":      (*conn).cmdStrLen,
		"GETRANGE":    (*conn).cmdGetRange,
		"SETRANGE":    (*conn).cmdSetRange,
		"INCR":        (*conn).cmdIncr,
		"DECR":        (*conn).cmdDecr,
		"INCRBY":      (*conn).cmdIncrBy,
		"DECRBY":      (*conn).cmdDecrBy,
		"INCRBYFLOAT": (*conn).cmdIncrByFloat,
		"MGET":        (*conn).cmdMGet,
		"MSET":        (*conn).cmdMSet,
		"COPY":        (*conn).cmdCopy,

		// hashes
		"HSET":         (*conn).cmdHSet,
		"HSETNX":       (*conn).cmdHSetNX,
		"HINCRBY":      (*conn).cmdHIncrBy,
		"HINCRBYFLOAT": (*conn).cmdHIncrByFloat,
		"HGET":         (*conn).cmdHGet,
		"HDEL":         (*conn).cmdHDel,
		"HGETALL":      (*conn).cmdHGetAll,
		"HKEYS":        (*conn).cmdHKeys,
		"HVALS":        (*conn).cmdHVals,
		"HLEN":         (*conn).cmdHLen,
		"HEXISTS":      (*conn).cmdHExists,
		"HMGET":        (*conn).cmdHMGet,

		// lists
		"LPUSH":   (*conn).cmdLPush,
		"RPUSH":   (*conn).cmdRPush,
		"LPOP":    (*conn).cmdLPop,
		"RPOP":    (*conn).cmdRPop,
		"LLEN":    (*conn).cmdLLen,
		"LINDEX":  (*conn).cmdLIndex,
		"LRANGE":  (*conn).cmdLRange,
		"LSET":    (*conn).cmdLSet,
		"LREM":    (*conn).cmdLRem,
		"LTRIM":   (*conn).cmdLTrim,
		"LINSERT": (*conn).cmdLInsert,
		"SORT":    (*conn).cmdSort,

		// sets
		"SADD":      (*conn).cmdSAdd,
		"SREM":      (*conn).cmdSRem,
		"SMEMBERS":  (*conn).cmdSMembers,
		"SISMEMBER": (*conn).cmdSIsMember,
		"SCARD":     (*conn).cmdSCard,

		// sorted sets
		"ZADD":          (*conn).cmdZAdd,
		"ZREM":          (*conn).cmdZRem,
		"ZSCORE":        (*conn).cmdZScore,
		"ZCARD":         (*conn).cmdZCard,
		"ZRANGE":        (*conn).cmdZRange,
		"ZRANGEBYSCORE": (*conn).cmdZRangeByScore,

		// pub/sub
		"SUBSCRIBE":   (*conn).cmdSubscribe,
		"UNSUBSCRIBE": (*conn).cmdUnsubscribe,
		"PUBLISH":     (*conn).cmdPublish,

		// vector store
		"VSET":    (*conn).cmdVSet,
		"VSEARCH": (*conn).cmdVSearch,
		"VDEL":    (*conn).cmdVDel,
		"VCARD":   (*conn).cmdVCard,
		"VDIM":    (*conn).cmdVDim,

		// semantic cache (dotted names, Redis-module style)
		"SCACHE.SET": (*conn).cmdSCacheSet,
		"SCACHE.GET": (*conn).cmdSCacheGet,

		// agent memory
		"REMEMBER": (*conn).cmdRemember,
		"RECALL":   (*conn).cmdRecall,
	}
}

// wrongArgs writes the standard arity error.
func (c *conn) wrongArgs(cmd string) error {
	return c.writeError(fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(cmd)))
}

// storeErr maps a store error to the appropriate RESP error reply.
func (c *conn) storeErr(err error) error {
	return c.writeError(err.Error())
}

func (c *conn) cmdPing(args []string) error {
	if len(args) >= 2 {
		return c.writeBulk(args[1])
	}
	return c.writeSimple("PONG")
}

func (c *conn) cmdEcho(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("echo")
	}
	return c.writeBulk(args[1])
}

func (c *conn) cmdQuit(args []string) error {
	c.writeSimple("OK")
	c.flush()
	c.nc.Close()
	return nil
}

// cmdSelect accepts only DB 0; RAMen is single-database in V1.
func (c *conn) cmdSelect(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("select")
	}
	if args[1] != "0" {
		return c.writeError("ERR DB index is out of range (RAMen V1 supports DB 0 only)")
	}
	return c.writeSimple("OK")
}

func (c *conn) cmdAuth(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("auth")
	}
	if c.s.cfg.Password == "" {
		return c.writeError("ERR Client sent AUTH, but no password is set")
	}
	if args[1] != c.s.cfg.Password {
		return c.writeError("WRONGPASS invalid username-password pair")
	}
	c.authed = true
	return c.writeSimple("OK")
}

// cmdCommand is a minimal stub: redis-cli issues COMMAND DOCS/COUNT on connect.
func (c *conn) cmdCommand(args []string) error {
	if len(args) >= 2 && strings.ToUpper(args[1]) == "COUNT" {
		return c.writeInt(int64(len(c.s.dispatch)))
	}
	return c.writeArray(0, nil)
}

func (c *conn) cmdDBSize(args []string) error {
	return c.writeInt(int64(c.s.store.DBSize()))
}

func (c *conn) cmdFlush(args []string) error {
	c.s.store.Flush()
	return c.writeSimple("OK")
}

func (c *conn) cmdSave(args []string) error {
	if c.s.cfg.Snapshotter == nil {
		return c.writeError("ERR persistence is disabled (no snapshot path configured)")
	}
	if err := c.s.cfg.Snapshotter.Save(); err != nil {
		return c.writeError("ERR " + err.Error())
	}
	return c.writeSimple("OK")
}

func (c *conn) cmdInfo(args []string) error {
	st := c.s.stats
	var b strings.Builder
	fmt.Fprintf(&b, "# Server\r\nramen_version:%s\r\ngo_version:%s\r\nuptime_in_seconds:%d\r\n",
		Version, runtime.Version(), int(c.s.Uptime().Seconds()))
	fmt.Fprintf(&b, "# Clients\r\nconnected_clients:%d\r\ntotal_connections_received:%d\r\n",
		st.Connections.Load(), st.TotalConns.Load())
	fmt.Fprintf(&b, "# Keyspace\r\ndb0:keys=%d\r\n", c.s.store.DBSize())
	fmt.Fprintf(&b, "# Stats\r\ntotal_commands_processed:%d\r\nsemantic_cache_hits:%d\r\nsemantic_cache_misses:%d\r\nsemantic_cache_hit_ratio:%.4f\r\n",
		st.Commands.Load(), st.CacheHits.Load(), st.CacheMisses.Load(), st.HitRatio())
	return c.writeBulk(b.String())
}

// --- generic keyspace ------------------------------------------------------

func (c *conn) cmdDel(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("del")
	}
	return c.writeInt(int64(c.s.store.Del(args[1:]...)))
}

func (c *conn) cmdExists(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("exists")
	}
	return c.writeInt(int64(c.s.store.Exists(args[1:]...)))
}

func (c *conn) cmdExpire(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("expire")
	}
	secs, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	ok := c.s.store.Expire(args[1], time.Duration(secs)*time.Second)
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdPExpire(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("pexpire")
	}
	ms, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	ok := c.s.store.Expire(args[1], time.Duration(ms)*time.Millisecond)
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdExpireAt(args []string) error  { return c.expireAt(args, "expireat", false) }
func (c *conn) cmdPExpireAt(args []string) error { return c.expireAt(args, "pexpireat", true) }

// maxExpireSeconds mirrors Redis' LLONG_MAX/1000 cap on an absolute second
// timestamp. A larger value would overflow time.Unix and wrap into the past,
// which would silently delete the key instead of setting a deadline.
const maxExpireSeconds = 9223372036854775

// expireAt sets an absolute deadline from a Unix timestamp given in seconds, or
// milliseconds when ms is true.
func (c *conn) expireAt(args []string, name string, ms bool) error {
	if len(args) != 3 {
		return c.wrongArgs(name)
	}
	ts, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return c.writeError(store.ErrNotInteger.Error())
	}
	var at time.Time
	if ms {
		at = time.UnixMilli(ts)
	} else {
		if ts > maxExpireSeconds || ts < -maxExpireSeconds {
			return c.writeError("ERR invalid expire time in '" + name + "' command")
		}
		at = time.Unix(ts, 0)
	}
	ok := c.s.store.ExpireAt(args[1], at)
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdTTL(args []string) error  { return c.ttl(args, time.Second) }
func (c *conn) cmdPTTL(args []string) error { return c.ttl(args, time.Millisecond) }

func (c *conn) ttl(args []string, unit time.Duration) error {
	if len(args) != 2 {
		return c.wrongArgs("ttl")
	}
	d, hasTTL, ok := c.s.store.TTL(args[1])
	if !ok {
		return c.writeInt(-2) // key does not exist
	}
	if !hasTTL {
		return c.writeInt(-1) // no associated expiry
	}
	return c.writeInt(int64(d / unit))
}

func (c *conn) cmdExpireTime(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("expiretime")
	}
	at, hasTTL, ok := c.s.store.ExpireTime(args[1])
	if !ok {
		return c.writeInt(-2) // key does not exist
	}
	if !hasTTL {
		return c.writeInt(-1) // no associated expiry
	}
	return c.writeInt(at.Unix())
}

func (c *conn) cmdPersist(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("persist")
	}
	return c.writeInt(boolToInt(c.s.store.Persist(args[1])))
}

func (c *conn) cmdKeys(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("keys")
	}
	return c.writeStringArray(c.s.store.Keys(args[1]))
}

func (c *conn) cmdType(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("type")
	}
	return c.writeSimple(c.s.store.Type(args[1]))
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
