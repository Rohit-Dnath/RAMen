package server

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/Rohit-Dnath/RAMen/internal/pubsub"
	"github.com/Rohit-Dnath/RAMen/internal/resp"
)

// respWriter is the raw RESP writer passed to writeArray callbacks so command
// handlers can emit nested array elements without importing the resp package.
type respWriter = *resp.Writer

// conn holds per-connection state. Writes are guarded by wmu because pub/sub
// delivery runs on a separate goroutine concurrently with command replies.
type conn struct {
	s   *Server
	nc  net.Conn
	r   *resp.Reader
	w   *resp.Writer
	wmu sync.Mutex

	authed bool

	submu sync.Mutex
	subs  map[string]*pubsub.Subscription
}

// serveConn runs the read/dispatch loop for a single client connection.
func (s *Server) serveConn(ctx context.Context, nc net.Conn) {
	defer nc.Close()
	s.stats.Connections.Add(1)
	s.stats.TotalConns.Add(1)
	defer s.stats.Connections.Add(-1)

	c := &conn{
		s:      s,
		nc:     nc,
		r:      resp.NewReader(nc),
		w:      resp.NewWriter(nc),
		subs:   make(map[string]*pubsub.Subscription),
		authed: s.cfg.Password == "",
	}
	defer c.unsubscribeAll()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		args, err := c.r.ReadCommand()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				// Malformed input or reset connection: nothing more to do.
			}
			return
		}
		if len(args) == 0 {
			continue
		}
		if err := s.dispatchCommand(c, args); err != nil {
			return // fatal write error
		}
		if err := c.flush(); err != nil {
			return
		}
	}
}

// --- guarded write helpers -------------------------------------------------

func (c *conn) flush() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.Flush()
}

func (c *conn) writeSimple(s string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteSimpleString(s)
}

func (c *conn) writeError(msg string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteError(msg)
}

func (c *conn) writeInt(n int64) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteInteger(n)
}

func (c *conn) writeBulk(s string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteBulkString(s)
}

func (c *conn) writeNull() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteNull()
}

func (c *conn) writeStringArray(items []string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.w.WriteStringArray(items)
}

// writeArray runs fn while holding the write lock, after emitting an array
// header of n elements. fn writes the elements using the raw writer.
func (c *conn) writeArray(n int, fn func(w *resp.Writer) error) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.w.WriteArrayHeader(n); err != nil {
		return err
	}
	return fn(c.w)
}

func (c *conn) unsubscribeAll() {
	c.submu.Lock()
	defer c.submu.Unlock()
	for ch, sub := range c.subs {
		c.s.broker.Unsubscribe(sub)
		delete(c.subs, ch)
	}
}
