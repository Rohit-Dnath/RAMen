package server

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// scacheCollection is the internal vector collection key backing the semantic
// cache. It is a normal vector collection, so it is snapshotted like any other
// key and visible (read-only in spirit) in the dashboard.
const scacheCollection = "__scache__"

// defaultThreshold is the minimum cosine similarity for a semantic-cache hit
// when the caller does not specify THRESHOLD.
const defaultThreshold = 0.90

// scacheMeta is what we stash in each cached vector's metadata.
type scacheMeta struct {
	Response   string `json:"r"`
	ExpireUnix int64  `json:"exp"` // 0 == no expiry
}

// embedPrompt turns prompt text into a vector via the configured provider.
func (c *conn) embedPrompt(prompt string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.s.cfg.Embed.Embed(ctx, prompt)
}

// cmdSCacheSet implements SCACHE.SET prompt response [TTL seconds].
func (c *conn) cmdSCacheSet(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("scache.set")
	}
	prompt, response := args[1], args[2]
	var expireUnix int64
	if len(args) >= 5 && strings.ToUpper(args[3]) == "TTL" {
		secs, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil {
			return c.writeError("ERR TTL must be an integer number of seconds")
		}
		expireUnix = time.Now().Add(time.Duration(secs) * time.Second).Unix()
	} else if len(args) != 3 {
		return c.writeError("ERR syntax error")
	}

	vec, err := c.embedPrompt(prompt)
	if err != nil {
		return c.writeError("ERR " + err.Error())
	}
	meta, _ := json.Marshal(scacheMeta{Response: response, ExpireUnix: expireUnix})
	if err := c.s.store.VSet(scacheCollection, prompt, vec, string(meta)); err != nil {
		return c.storeErr(err)
	}
	return c.writeSimple("OK")
}

// cmdSCacheGet implements SCACHE.GET prompt [THRESHOLD x]. On a hit it returns
// the cached response (bulk string); on a miss it returns null. Hit/miss
// counters feed INFO and the dashboard.
func (c *conn) cmdSCacheGet(args []string) error {
	if len(args) != 2 && len(args) != 4 {
		return c.wrongArgs("scache.get")
	}
	prompt := args[1]
	threshold := defaultThreshold
	if len(args) == 4 {
		if strings.ToUpper(args[2]) != "THRESHOLD" {
			return c.writeError("ERR syntax error")
		}
		t, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			return c.writeError("ERR THRESHOLD must be a float")
		}
		threshold = t
	}

	vec, err := c.embedPrompt(prompt)
	if err != nil {
		return c.writeError("ERR " + err.Error())
	}
	results, err := c.s.store.VSearch(scacheCollection, vec, 1)
	if err != nil {
		return c.storeErr(err)
	}
	if len(results) == 0 || results[0].Score < threshold {
		c.s.stats.CacheMisses.Add(1)
		return c.writeNull()
	}

	var meta scacheMeta
	if err := json.Unmarshal([]byte(results[0].Item.Meta), &meta); err != nil {
		c.s.stats.CacheMisses.Add(1)
		return c.writeNull()
	}
	if meta.ExpireUnix != 0 && time.Now().Unix() > meta.ExpireUnix {
		// Expired: drop it and report a miss.
		c.s.store.VDel(scacheCollection, results[0].Item.ID)
		c.s.stats.CacheMisses.Add(1)
		return c.writeNull()
	}
	c.s.stats.CacheHits.Add(1)
	return c.writeBulk(meta.Response)
}
