// Package server is RAMen's RESP2 TCP server: it accepts connections, parses
// commands, dispatches them against the store, and writes replies. One
// goroutine serves each connection, which maps cleanly onto Go's concurrency
// model (PRD §9).
package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/embed"
	"github.com/Rohit-Dnath/RAMen/internal/persist"
	"github.com/Rohit-Dnath/RAMen/internal/pubsub"
	"github.com/Rohit-Dnath/RAMen/internal/store"
)

// Config configures a Server.
type Config struct {
	Addr        string // listen address, e.g. ":6379"
	Password    string // optional AUTH password ("" disables auth)
	Snapshotter *persist.Snapshotter
	Embed       *embed.Client
}

// Server holds shared state for all connections.
type Server struct {
	cfg     Config
	store   *store.Store
	broker  *pubsub.Broker
	stats   *Stats
	started time.Time

	ln       net.Listener
	wg       sync.WaitGroup
	dispatch map[string]handler
}

// handler executes one command. It returns a non-nil error only on a fatal I/O
// problem (e.g. the connection died); command-level failures are written to
// the client as RESP errors.
type handler func(c *conn, args []string) error

// New builds a Server over the given store.
func New(s *store.Store, cfg Config) *Server {
	srv := &Server{
		cfg:     cfg,
		store:   s,
		broker:  pubsub.NewBroker(),
		stats:   &Stats{},
		started: time.Now(),
	}
	srv.registerCommands()
	return srv
}

// Store exposes the underlying keyspace (used by the dashboard).
func (s *Server) Store() *store.Store { return s.store }

// Stats exposes server counters (used by the dashboard).
func (s *Server) Stats() *Stats { return s.stats }

// Uptime returns how long the server has been running.
func (s *Server) Uptime() time.Duration { return time.Since(s.started) }

// ListenAndServe binds the configured address and serves connections until ctx
// is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}
	s.ln = ln
	fmt.Printf("ramen: listening on %s\n", ln.Addr())

	go func() {
		<-ctx.Done()
		ln.Close() // unblocks Accept
	}()

	for {
		nc, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return nil // clean shutdown
			default:
				fmt.Fprintf(os.Stderr, "ramen: accept error: %v\n", err)
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(ctx, nc)
		}()
	}
}

// dispatchCommand looks up and runs a command by name.
func (s *Server) dispatchCommand(c *conn, args []string) error {
	if len(args) == 0 {
		return nil
	}
	s.stats.Commands.Add(1)
	name := strings.ToUpper(args[0])

	// multiple length commands
	switch strings.ToUpper(name) {
	case "OBJECT":
		if len(args) > 1 {
			name += " " + strings.ToUpper(args[1])
		}
	}

	// When not authenticated, only AUTH (and QUIT) are permitted.
	if s.cfg.Password != "" && !c.authed && name != "AUTH" && name != "QUIT" {
		return c.writeError("NOAUTH Authentication required.")
	}

	h, ok := s.dispatch[name]
	if !ok {
		return c.writeError(fmt.Sprintf("ERR unknown command '%s'", args[0]))
	}
	return h(c, args)
}
