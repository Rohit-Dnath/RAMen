// Package client is a tiny RESP2 client. It exists so the built-in MCP server
// (and tests) can talk to a running RAMen instance over the same wire protocol
// any Redis client uses, rather than reaching into the store directly.
package client

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client is a single synchronous connection to a RAMen/Redis server. It is not
// safe for concurrent use; guard it with a mutex if shared.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to addr (host:port).
func Dial(addr string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, r: bufio.NewReader(conn)}, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// RemoteAddr returns the server address this client is connected to.
func (c *Client) RemoteAddr() string { return c.conn.RemoteAddr().String() }

// Do sends a command as a RESP array of bulk strings and returns the decoded
// reply. The reply is one of: string, int64, nil, []any, or an error value
// representing a RESP error.
func (c *Client) Do(args ...string) (any, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := c.conn.Write([]byte(b.String())); err != nil {
		return nil, err
	}
	return c.readReply()
}

func (c *Client) readReply() (any, error) {
	prefix, err := c.r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := c.readLine()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return fmt.Errorf("%s", line), nil
	case ':':
		return strconv.ParseInt(line, 10, 64)
	case '$':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		arr := make([]any, n)
		for i := 0; i < n; i++ {
			if arr[i], err = c.readReply(); err != nil {
				return nil, err
			}
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("client: unknown reply type %q", prefix)
	}
}

func (c *Client) readLine() (string, error) {
	s, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}
