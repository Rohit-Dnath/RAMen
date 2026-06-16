package server

// Agent-memory primitives (PRD §7): session-scoped key namespaces. They are a
// thin convenience layer over hashes — REMEMBER/RECALL store and fetch facts
// under a per-session hash named "mem:<session>" — and are also exposed as the
// MCP `remember` tool so an agent can persist context across turns.

const memPrefix = "mem:"

func memKey(session string) string { return memPrefix + session }

// cmdRemember implements REMEMBER session field value.
func (c *conn) cmdRemember(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("remember")
	}
	if _, err := c.s.store.HSet(memKey(args[1]), map[string]string{args[2]: args[3]}); err != nil {
		return c.storeErr(err)
	}
	return c.writeSimple("OK")
}

// cmdRecall implements RECALL session [field]. With a field it returns that
// value (or null); without one it returns the whole session as a flat
// [field, value, ...] array.
func (c *conn) cmdRecall(args []string) error {
	if len(args) != 2 && len(args) != 3 {
		return c.wrongArgs("recall")
	}
	if len(args) == 3 {
		v, ok, err := c.s.store.HGet(memKey(args[1]), args[2])
		if err != nil {
			return c.storeErr(err)
		}
		if !ok {
			return c.writeNull()
		}
		return c.writeBulk(v)
	}
	flat, err := c.s.store.HGetAll(memKey(args[1]))
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(flat)
}
