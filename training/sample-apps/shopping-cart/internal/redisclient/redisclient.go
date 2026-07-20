// Package redisclient is a deliberately tiny, dependency-free Redis client that
// speaks just enough of the RESP protocol (https://redis.io/docs/reference/protocol-spec)
// for the shopping-cart backend: PING, SET, GET, LPUSH and LRANGE.
//
// It exists so the training app stays standard-library only (no
// github.com/redis/go-redis) while still demonstrating a real network
// dependency that Ring Promoter's readiness checks can watch. Every call dials a
// fresh connection with a short timeout, so a missing or down Redis surfaces as
// a plain error the caller can treat gracefully — the app never crashes.
package redisclient

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client talks to a single Redis endpoint over TCP.
type Client struct {
	addr    string
	timeout time.Duration
}

// New returns a client for addr (host:port). It does not connect until a command
// is issued.
func New(addr string) *Client {
	return &Client{addr: addr, timeout: 2 * time.Second}
}

// Addr reports the configured endpoint.
func (c *Client) Addr() string { return c.addr }

// Do sends one command and returns the decoded reply. Reply types map to Go as:
// simple string / bulk string -> string, integer -> int64, nil bulk -> nil,
// array -> []any, Redis error -> error.
func (c *Client) Do(args ...string) (any, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	// Encode the command as a RESP array of bulk strings.
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return nil, err
	}
	return readReply(bufio.NewReader(conn))
}

// Ping verifies the server answers.
func (c *Client) Ping() error {
	_, err := c.Do("PING")
	return err
}

// Set stores a string value.
func (c *Client) Set(key, value string) error {
	_, err := c.Do("SET", key, value)
	return err
}

// Get returns a stored value, or "" when the key is missing.
func (c *Client) Get(key string) (string, error) {
	reply, err := c.Do("GET", key)
	if err != nil {
		return "", err
	}
	if reply == nil {
		return "", nil
	}
	s, ok := reply.(string)
	if !ok {
		return "", fmt.Errorf("redisclient: unexpected GET reply %T", reply)
	}
	return s, nil
}

// LPush prepends a value to a list and returns the new length.
func (c *Client) LPush(key, value string) (int64, error) {
	reply, err := c.Do("LPUSH", key, value)
	if err != nil {
		return 0, err
	}
	n, ok := reply.(int64)
	if !ok {
		return 0, fmt.Errorf("redisclient: unexpected LPUSH reply %T", reply)
	}
	return n, nil
}

// LRange returns the elements of a list in the inclusive range [start, stop].
func (c *Client) LRange(key string, start, stop int) ([]string, error) {
	reply, err := c.Do("LRANGE", key, strconv.Itoa(start), strconv.Itoa(stop))
	if err != nil {
		return nil, err
	}
	arr, ok := reply.([]any)
	if !ok {
		return nil, fmt.Errorf("redisclient: unexpected LRANGE reply %T", reply)
	}
	out := make([]string, 0, len(arr))
	for _, el := range arr {
		s, ok := el.(string)
		if !ok {
			return nil, fmt.Errorf("redisclient: unexpected LRANGE element %T", el)
		}
		out = append(out, s)
	}
	return out, nil
}

// readReply decodes a single RESP reply.
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
		return nil, errors.New(line)
	case ':': // integer
		return strconv.ParseInt(line, 10, 64)
	case '$': // bulk string
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil // nil bulk
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
			return nil, nil // nil array
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			el, err := readReply(r)
			if err != nil {
				return nil, err
			}
			out = append(out, el)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("redisclient: unknown reply prefix %q", prefix)
	}
}

// readLine reads up to a CRLF and returns the content without it.
func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
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
