package server

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Rohit-Dnath/RAMen/internal/store"
)

// errInvalidCursor is the reply Redis gives when the cursor is not a valid
// unsigned integer. errSyntax is the generic option-parsing error.
var (
	errInvalidCursor = errors.New("ERR invalid cursor")
	errSyntax        = errors.New("ERR syntax error")
)

// scanArgs holds the parsed cursor and the optional MATCH/COUNT arguments shared
// by SCAN, HSCAN, SSCAN and ZSCAN.
type scanArgs struct {
	cursor uint64
	match  string // "" means match everything
	count  int    // 0 means use the store default
}

// parseScanTail reads the cursor at args[cursorIdx] and then the trailing
// [MATCH pattern] [COUNT count] options. COUNT is a hint and must be positive;
// anything else is a syntax error, matching Redis.
func parseScanTail(args []string, cursorIdx int) (scanArgs, error) {
	var out scanArgs
	cur, err := strconv.ParseUint(args[cursorIdx], 10, 64)
	if err != nil {
		return out, errInvalidCursor
	}
	out.cursor = cur
	for i := cursorIdx + 1; i < len(args); {
		switch strings.ToUpper(args[i]) {
		case "MATCH":
			if i+1 >= len(args) {
				return out, errSyntax
			}
			out.match = args[i+1]
			i += 2
		case "COUNT":
			if i+1 >= len(args) {
				return out, errSyntax
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return out, store.ErrNotInteger
			}
			if n < 1 {
				return out, errSyntax
			}
			out.count = n
			i += 2
		default:
			return out, errSyntax
		}
	}
	return out, nil
}

// writeScanReply emits the two-element SCAN reply: the next cursor as a bulk
// string followed by the array of elements.
func (c *conn) writeScanReply(cursor uint64, elements []string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.w.WriteArrayHeader(2); err != nil {
		return err
	}
	if err := c.w.WriteBulkString(strconv.FormatUint(cursor, 10)); err != nil {
		return err
	}
	return c.w.WriteStringArray(elements)
}

// cmdScan implements SCAN cursor [MATCH pattern] [COUNT count].
func (c *conn) cmdScan(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("scan")
	}
	opts, err := parseScanTail(args, 1)
	if err != nil {
		return c.writeError(err.Error())
	}
	next, keys := c.s.store.Scan(opts.cursor, opts.match, opts.count)
	return c.writeScanReply(next, keys)
}

// cmdHScan implements HSCAN key cursor [MATCH pattern] [COUNT count]. The
// elements are a flat [field, value, ...] list.
func (c *conn) cmdHScan(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("hscan")
	}
	opts, err := parseScanTail(args, 2)
	if err != nil {
		return c.writeError(err.Error())
	}
	next, pairs, err := c.s.store.HScan(args[1], opts.cursor, opts.match, opts.count)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeScanReply(next, pairs)
}

// cmdSScan implements SSCAN key cursor [MATCH pattern] [COUNT count].
func (c *conn) cmdSScan(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("sscan")
	}
	opts, err := parseScanTail(args, 2)
	if err != nil {
		return c.writeError(err.Error())
	}
	next, members, err := c.s.store.SScan(args[1], opts.cursor, opts.match, opts.count)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeScanReply(next, members)
}

// cmdZScan implements ZSCAN key cursor [MATCH pattern] [COUNT count]. The
// elements are a flat [member, score, ...] list, scores formatted like ZSCORE.
func (c *conn) cmdZScan(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("zscan")
	}
	opts, err := parseScanTail(args, 2)
	if err != nil {
		return c.writeError(err.Error())
	}
	next, members, err := c.s.store.ZScan(args[1], opts.cursor, opts.match, opts.count)
	if err != nil {
		return c.storeErr(err)
	}
	flat := make([]string, 0, len(members)*2)
	for _, m := range members {
		flat = append(flat, m.Member, formatFloat(m.Score))
	}
	return c.writeScanReply(next, flat)
}
