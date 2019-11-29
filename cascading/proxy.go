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

// const defaultSize = 4
//
// type proxy struct {
// 	queue  chan net.Conn
// 	buffer bytes.Buffer
//
// 	addr string
// }
//
// func Proxy(addr string, n int) (io.WriteCloser, error) {
// 	if _, _, err := net.SplitHostPort(addr); err != nil {
// 		return nil, err
// 	}
// 	if n <= 0 {
// 		n = defaultSize
// 	}
// 	p := proxy{
// 		addr:  addr,
// 		queue: make(chan net.Conn, n),
// 	}
// 	return &p, nil
// }
//
// func (p *proxy) Close() error {
// 	err := p.flush()
// 	for c := range p.queue {
// 		c.Close()
// 	}
// 	return err
// }
//
// var syncword = []byte{0xf8, 0x2e, 0x35, 0x53}
//
// func (p *proxy) Write(bs []byte) (int, error) {
// 	if len(bs) == 0 || (bytes.HasPrefix(bs, syncword) && p.buffer.Len() > 0) {
// 		defer p.buffer.Reset()
//
// 		c, err := p.pop()
// 		if err != nil {
// 			return 0, nil
// 		}
//
// 		xs, _ := ioutil.ReadAll(&p.buffer)
// 		go func(c net.Conn, xs []byte) {
// 			if _, err := c.Write(xs); err != nil {
// 				c.Close()
// 			} else {
// 				p.push(c)
// 			}
// 		}(c, xs)
// 	}
//
// 	return p.buffer.Write(bs)
// }
//
// func (p *proxy) flush() error {
// 	var err error
// 	if p.buffer.Len() > 0 {
// 		_, err = p.Write(nil)
// 	}
// 	return err
// }
//
// func (p *proxy) pop() (net.Conn, error) {
// 	select {
// 	case c := <-p.queue:
// 		return c, nil
// 	default:
// 		return client(p.addr)
// 	}
// }
//
// func (p *proxy) push(c net.proxy) {
// 	select {
// 	case p.queue <- c:
// 	default:
// 		c.Close()
// 	}
// }
//
// func client(addr string) (net.Conn, error) {
// 	// c, err := net.Dial("tcp", addr)
// 	c, err := net.DialTimeout("tcp", addr, time.Second)
// 	if err != nil {
// 		return nil, err
// 	}
// 	n := conn{
// 		Conn:   c,
// 		writer: c,
// 		addr:   addr,
// 	}
// 	return &n, nil
// }
//
// type conn struct {
// 	net.Conn
// 	writer io.Writer
//
// 	addr string
// }
//
// func (c *proxy) Write(bs []byte) (int, error) {
// 	return c.writer.Write(bs)
// }
