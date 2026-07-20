// Package redis is a deliberately tiny, dependency-free Redis client. It speaks
// just enough of the RESP protocol (https://redis.io/docs/reference/protocol-spec)
// over a raw TCP connection to run the handful of commands image-proc needs:
// PING, LPUSH, RPOP, BRPOP, LLEN, SET and GET.
//
// It exists so the training apps stay standard-library only (no module
// dependencies) while still demonstrating a real queue-backed, async workload.
// It is NOT production grade: one connection guarded by a mutex, lazy connect
// with reconnect-on-error. That is plenty for a demo and keeps the moving parts
// visible.
package redis

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Nil is returned (wrapped) when Redis replies with a null bulk string, e.g.
// GET on a missing key or a timed-out BRPOP.
var Nil = errors.New("redis: nil")

// Client is a minimal single-connection Redis client. Safe for concurrent use:
// every command is serialized by mu, and a broken connection is transparently
// re-dialed on the next command.
type Client struct {
	addr string

	mu   sync.Mutex
	conn net.Conn
	rw   *bufio.ReadWriter
}

// New returns a client for addr (host:port). It does not dial until the first
// command, so constructing a client never fails even if Redis is down.
func New(addr string) *Client {
	return &Client{addr: addr}
}

func (c *Client) connect() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	c.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	return nil
}

func (c *Client) drop() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.rw = nil
	}
}

// do sends one command and reads one reply, holding the lock for the round
// trip. On any I/O error it drops the connection so the next call reconnects.
func (c *Client) do(timeout time.Duration, args ...string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.connect(); err != nil {
		return nil, err
	}

	// A per-command deadline keeps a dead peer from hanging the caller. For
	// blocking reads (BRPOP) the caller passes a longer timeout.
	deadline := time.Now().Add(timeout)
	_ = c.conn.SetDeadline(deadline)

	if err := writeCommand(c.rw.Writer, args); err != nil {
		c.drop()
		return nil, err
	}
	if err := c.rw.Writer.Flush(); err != nil {
		c.drop()
		return nil, err
	}

	reply, err := readReply(c.rw.Reader)
	if err != nil {
		// A RESP error reply (e.g. WRONGTYPE) is a valid protocol response, not
		// a transport failure, so keep the connection alive for those.
		var rerr *Error
		if !errors.As(err, &rerr) && !errors.Is(err, Nil) {
			c.drop()
		}
		return nil, err
	}
	return reply, nil
}

// Error is a server-side RESP error reply ("-ERR ...").
type Error struct{ Msg string }

func (e *Error) Error() string { return "redis: " + e.Msg }

// Ping returns nil if the server answers PONG.
func (c *Client) Ping() error {
	_, err := c.do(3*time.Second, "PING")
	return err
}

// LPush pushes value onto the head of list key and returns the new length.
func (c *Client) LPush(key, value string) (int64, error) {
	r, err := c.do(3*time.Second, "LPUSH", key, value)
	if err != nil {
		return 0, err
	}
	return toInt(r)
}

// RPop pops one value from the tail of list key. Returns Nil if the list is
// empty.
func (c *Client) RPop(key string) (string, error) {
	r, err := c.do(3*time.Second, "RPOP", key)
	if err != nil {
		return "", err
	}
	return toString(r)
}

// BRPop blocks up to timeout for a value on the tail of key. Returns Nil on
// timeout. The socket deadline is set a little beyond the server-side timeout.
func (c *Client) BRPop(key string, timeout time.Duration) (string, error) {
	secs := int(timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	r, err := c.do(timeout+3*time.Second, "BRPOP", key, strconv.Itoa(secs))
	if err != nil {
		return "", err
	}
	// BRPOP replies with a 2-element array [key, value], or nil on timeout.
	arr, ok := r.([]any)
	if !ok || len(arr) != 2 {
		return "", Nil
	}
	return toString(arr[1])
}

// LLen returns the length of list key (0 if it does not exist).
func (c *Client) LLen(key string) (int64, error) {
	r, err := c.do(3*time.Second, "LLEN", key)
	if err != nil {
		return 0, err
	}
	return toInt(r)
}

// Set sets key to value.
func (c *Client) Set(key, value string) error {
	_, err := c.do(3*time.Second, "SET", key, value)
	return err
}

// Get returns the value at key. Returns Nil if the key is missing.
func (c *Client) Get(key string) (string, error) {
	r, err := c.do(3*time.Second, "GET", key)
	if err != nil {
		return "", err
	}
	return toString(r)
}

// --- RESP encoding/decoding ---

func writeCommand(w *bufio.Writer, args []string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(a), a); err != nil {
			return err
		}
	}
	return nil
}

func readReply(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+': // simple string
		return line, nil
	case '-': // error
		return nil, &Error{Msg: line}
	case ':': // integer
		return strconv.ParseInt(line, 10, 64)
	case '$': // bulk string
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, Nil
		}
		buf := make([]byte, n+2) // include trailing CRLF
		if _, err := readFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*': // array
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, Nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			elem, err := readReply(r)
			if err != nil && !errors.Is(err, Nil) {
				return nil, err
			}
			if errors.Is(err, Nil) {
				out = append(out, nil)
				continue
			}
			out = append(out, elem)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("redis: unknown reply type %q", prefix)
	}
}

// readLine reads through the next CRLF and returns the line without it.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Trim trailing \r\n (or just \n).
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func toInt(v any) (int64, error) {
	if i, ok := v.(int64); ok {
		return i, nil
	}
	return 0, fmt.Errorf("redis: expected integer, got %T", v)
}

func toString(v any) (string, error) {
	switch s := v.(type) {
	case string:
		return s, nil
	case nil:
		return "", Nil
	default:
		return "", fmt.Errorf("redis: expected string, got %T", v)
	}
}
