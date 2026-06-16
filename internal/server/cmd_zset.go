package server

import (
	"strconv"
	"strings"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

func (c *conn) cmdZAdd(args []string) error {
	// ZADD key score member [score member ...]
	if len(args) < 4 || len(args)%2 != 0 {
		return c.wrongArgs("zadd")
	}
	members := make([]store.ZMember, 0, (len(args)-2)/2)
	for i := 2; i+1 < len(args); i += 2 {
		score, err := strconv.ParseFloat(args[i], 64)
		if err != nil {
			return c.writeError("ERR value is not a valid float")
		}
		members = append(members, store.ZMember{Member: args[i+1], Score: score})
	}
	n, err := c.s.store.ZAdd(args[1], members)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdZRem(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("zrem")
	}
	n, err := c.s.store.ZRem(args[1], args[2:]...)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdZScore(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("zscore")
	}
	score, ok, err := c.s.store.ZScore(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(formatFloat(score))
}

func (c *conn) cmdZCard(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("zcard")
	}
	n, err := c.s.store.ZCard(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdZRange(args []string) error {
	// ZRANGE key start stop [WITHSCORES]
	if len(args) != 4 && len(args) != 5 {
		return c.wrongArgs("zrange")
	}
	start, err1 := strconv.Atoi(args[2])
	stop, err2 := strconv.Atoi(args[3])
	if err1 != nil || err2 != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	withScores := len(args) == 5 && strings.ToUpper(args[4]) == "WITHSCORES"
	if len(args) == 5 && !withScores {
		return c.writeError("ERR syntax error")
	}
	members, err := c.s.store.ZRange(args[1], start, stop)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeZMembers(members, withScores)
}

func (c *conn) cmdZRangeByScore(args []string) error {
	// ZRANGEBYSCORE key min max [WITHSCORES]
	if len(args) != 4 && len(args) != 5 {
		return c.wrongArgs("zrangebyscore")
	}
	min, err1 := parseScoreBound(args[2])
	max, err2 := parseScoreBound(args[3])
	if err1 != nil || err2 != nil {
		return c.writeError("ERR min or max is not a float")
	}
	withScores := len(args) == 5 && strings.ToUpper(args[4]) == "WITHSCORES"
	if len(args) == 5 && !withScores {
		return c.writeError("ERR syntax error")
	}
	members, err := c.s.store.ZRangeByScore(args[1], min, max)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeZMembers(members, withScores)
}

// writeZMembers writes a flat array of members, interleaving scores when
// withScores is set (Redis layout: m1, s1, m2, s2, ...).
func (c *conn) writeZMembers(members []store.ZMember, withScores bool) error {
	n := len(members)
	if withScores {
		n *= 2
	}
	return c.writeArray(n, func(w respWriter) error {
		for _, m := range members {
			if err := w.WriteBulkString(m.Member); err != nil {
				return err
			}
			if withScores {
				if err := w.WriteBulkString(formatFloat(m.Score)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func parseScoreBound(s string) (float64, error) {
	switch strings.ToLower(s) {
	case "-inf":
		return -1e308, nil
	case "+inf", "inf":
		return 1e308, nil
	}
	return strconv.ParseFloat(s, 64)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
