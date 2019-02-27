package cascading

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
)

const defaultSize = 4

type proxy struct {
	queue  chan net.Conn
	buffer bytes.Buffer

	addr string
}

func Proxy(addr string, n int) (io.WriteCloser, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, err
	}
	if n <= 0 {
		n = defaultSize
	}
	p := proxy{
		addr:  addr,
		queue: make(chan net.Conn, n),
	}
	return &p, nil
}

func (p *proxy) Close() error {
	err := p.flush()
	for c := range p.queue {
		c.Close()
	}
	return err
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

	return p.buffer.Write(bs)
}

func (p *proxy) flush() error {
	var err error
	if p.buffer.Len() > 0 {
		_, err = p.Write(nil)
	}
	return err
}

func (p *proxy) pop() (net.Conn, error) {
	select {
	case c := <-p.queue:
		return c, nil
	default:
		return client(p.addr)
	}
}

func (p *proxy) push(c net.Conn) {
	select {
	case p.queue <- c:
	default:
		c.Close()
	}
}

func client(addr string) (net.Conn, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	n := conn{
		Conn:   c,
		writer: c,
		addr:   addr,
	}
	return &n, nil
}

type conn struct {
	net.Conn
	writer io.Writer

	addr string
}

func (c *conn) Write(bs []byte) (int, error) {
	return c.writer.Write(bs)
}
