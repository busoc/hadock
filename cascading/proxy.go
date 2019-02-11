package cascading

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net"
)

const (
	nogzip      = -10
	defaultSize = 4
)

type proxy struct {
	queue  chan net.Conn
	buffer bytes.Buffer

	addr  string
	level int
}

func Proxy(addr, level string, n int) (io.WriteCloser, error) {
	if n <= 0 {
		n = defaultSize
	}
	var gz int
	switch level {
	default:
		gz = nogzip
	case "no":
		gz = gzip.NoCompression
	case "speed":
		gz = gzip.BestSpeed
	case "best":
		gz = gzip.BestCompression
	case "default":
		gz = gzip.DefaultCompression
	}
	p := proxy{
		addr:  addr,
		level: gz,
		queue: make(chan net.Conn, n),
	}
	for i := 0; i < n; i++ {
		c, err := client(p.addr, p.level)
		if err != nil {
			return nil, err
		}
		p.push(c)
	}
	return &p, nil
}

func (p *proxy) Close() error {
	if p.buffer.Len() > 0 {
		p.Write(nil)
	}
	for c := range p.queue {
		c.Close()
	}
	return nil
}

var syncword = []byte{0xf8, 0x2e, 0x35, 0x53}

func (p *proxy) Write(bs []byte) (int, error) {
	if len(bs) == 0 || (bytes.HasPrefix(bs, syncword) && p.buffer.Len() > 0) {
		c, err := p.pop()
		if err != nil {
			return 0, nil
		}

		xs, _ := ioutil.ReadAll(&p.buffer)
		go func(c net.Conn, xs []byte) {
			if _, err := c.Write(xs); err != nil {
				c.Close()
			} else {
				p.push(c)
			}
		}(c, xs)
		p.buffer.Reset()
	}
	p.buffer.Write(bs)
	return len(bs), nil
}

func (p *proxy) pop() (net.Conn, error) {
	select {
	case c := <-p.queue:
		return c, nil
	default:
		return client(p.addr, p.level)
	}
}

func (p *proxy) push(c net.Conn) {
	select {
	case p.queue <- c:
	default:
		c.Close()
	}
}

func client(addr string, level int) (net.Conn, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	n := conn{Conn: c, writer: c}
	if level != nogzip {
		n.writer, _ = gzip.NewWriterLevel(n.writer, level)
	}
	return &n, nil
}

type conn struct {
	net.Conn
	writer io.Writer
}

func (c *conn) Write(bs []byte) (int, error) {
	_, err := c.writer.Write(bs)
	if err == nil {
		if f, ok := c.writer.(*gzip.Writer); ok {
			err = f.Flush()
		}
	}
	return len(bs), err
}
