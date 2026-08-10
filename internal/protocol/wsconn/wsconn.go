package wsconn

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/coder/websocket"
)

// Conn adapts a WebSocket to net.Conn using binary frames (for yamux).
type Conn struct {
	ctx  context.Context
	conn *websocket.Conn
	r    io.Reader
}

func New(ctx context.Context, conn *websocket.Conn) *Conn {
	return &Conn{ctx: ctx, conn: conn}
}

func (c *Conn) Read(b []byte) (int, error) {
	for {
		if c.r != nil {
			n, err := c.r.Read(b)
			if n > 0 || err != io.EOF {
				return n, err
			}
			c.r = nil
		}

		typ, reader, err := c.conn.Reader(c.ctx)
		if err != nil {
			return 0, err
		}
		if typ != websocket.MessageBinary {
			return 0, net.ErrClosed
		}
		c.r = reader
	}
}

func (c *Conn) Write(b []byte) (int, error) {
	if err := c.conn.Write(c.ctx, websocket.MessageBinary, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *Conn) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Conn) LocalAddr() net.Addr              { return dummyAddr{} }
func (c *Conn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (c *Conn) SetDeadline(t time.Time) error    { return nil }
func (c *Conn) SetReadDeadline(t time.Time) error  { return nil }
func (c *Conn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "ws" }
func (dummyAddr) String() string  { return "websocket" }
