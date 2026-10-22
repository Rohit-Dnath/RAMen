package server

import (
	"strconv"
	"strings"
)

// parseVector parses consecutive float arguments starting at args[i], stopping
// at the first recognised keyword (case-insensitive) or the end. It returns the
// vector and the index of the first unconsumed argument.
func parseVector(args []string, i int, stopWords ...string) ([]float32, int, error) {
	stop := make(map[string]bool, len(stopWords))
	for _, w := range stopWords {
		stop[w] = true
	}
	var vec []float32
	for ; i < len(args); i++ {
		if stop[strings.ToUpper(args[i])] {
			break
		}
		f, err := strconv.ParseFloat(args[i], 32)
		if err != nil {
			return nil, i, err
		}
		vec = append(vec, float32(f))
	}
	return vec, i, nil
}

// cmdVSet implements VSET key id f1 f2 ... fn [META <json-or-text>].
func (c *conn) cmdVSet(args []string) error {
	if len(args) < 4 {
		return c.wrongArgs("vset")
	}
	key, id := args[1], args[2]
	vec, i, err := parseVector(args, 3, "META")
	if err != nil {
		return c.writeError("ERR vector component is not a valid float")
	}
	if len(vec) == 0 {
		return c.writeError("ERR VSET requires at least one vector component")
	}
	meta := ""
	if i < len(args) && strings.ToUpper(args[i]) == "META" {
		if i+1 >= len(args) {
			return c.writeError("ERR META requires a value")
		}
		meta = args[i+1]
	}
	if err := c.s.store.VSet(key, id, vec, meta, 0); err != nil {
		return c.storeErr(err)
	}
	return c.writeSimple("OK")
}

// cmdVSearch implements VSEARCH key f1 ... fn [TOPK k] [WITHSCORES].
func (c *conn) cmdVSearch(args []string) error {
	if len(args) < 3 {
		return c.wrongArgs("vsearch")
	}
	key := args[1]
	vec, i, err := parseVector(args, 2, "TOPK", "WITHSCORES")
	if err != nil {
		return c.writeError("ERR vector component is not a valid float")
	}
	topK := 10
	withScores := false
	for ; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "TOPK":
			if i+1 >= len(args) {
				return c.writeError("ERR TOPK requires a value")
			}
			k, err := strconv.Atoi(args[i+1])
			if err != nil || k <= 0 {
				return c.writeError("ERR TOPK must be a positive integer")
			}
			topK = k
			i++
		case "WITHSCORES":
			withScores = true
		default:
			return c.writeError("ERR syntax error")
		}
	}
	results, err := c.s.store.VSearch(key, vec, topK)
	if err != nil {
		return c.storeErr(err)
	}
	// Reply: array of [id, meta] (and score when WITHSCORES) per hit.
	per := 2
	if withScores {
		per = 3
	}
	return c.writeArray(len(results)*per, func(w respWriter) error {
		for _, r := range results {
			if err := w.WriteBulkString(r.Item.ID); err != nil {
				return err
			}
			if err := w.WriteBulkString(r.Item.Meta); err != nil {
				return err
			}
			if withScores {
				if err := w.WriteBulkString(formatFloat(r.Score)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (c *conn) cmdVDel(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("vdel")
	}
	ok, err := c.s.store.VDel(args[1], args[2])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(boolToInt(ok))
}

func (c *conn) cmdVCard(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("vcard")
	}
	n, err := c.s.store.VCard(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdVDim(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("vdim")
	}
	n, err := c.s.store.VDim(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}
