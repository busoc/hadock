package cascading

import (
	// "bytes"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"
)

type proxy struct {
	net.Conn

	mu     sync.Mutex
	writer io.Writer
}

func Proxy(addr string, _ int) (io.WriteCloser, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, err
	}
	x, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := proxy{
		Conn:   x,
		writer: x,
	}
	return &c, nil
}

func (c *proxy) Write(bs []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.writer.Write(bs)
	if err != nil {
		switch c.Conn.(type) {
		case *net.UDPConn:
		case *net.TCPConn:
			c.writer = ioutil.Discard

			go c.reconnect()
		}
	}
	return len(bs), nil
}

func (c *proxy) reconnect() {
	addr := c.RemoteAddr().String()
	defer c.Conn.Close()
	for {
		x, err := net.DialTimeout("tcp", addr, time.Second*5)
		if err == nil {
			c.mu.Lock()
			defer c.mu.Unlock()

			c.writer, c.Conn = x, x
			break
		}
	}
}
