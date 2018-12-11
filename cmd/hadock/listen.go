package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"plugin"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/panda"
	"github.com/midbel/cli"
	"github.com/midbel/toml"
)

type module struct {
	Location string   `json:"location"`
	Config   []string `json:"config"`
}

type storer struct {
	Disabled    bool   `toml:"disabled"`
	Scheme      string `toml:"type"`
	Location    string `toml:"location"`
	Hard        string `toml:"link"`
	Raw         bool   `toml:"raw"`
	Remove      bool   `toml:"remove"`
	Granularity uint   `toml:"interval"`
}

type proxy struct {
	Addr  string `toml:"address"`
	Level string `toml:"level"`
}

type pool struct {
	Interval  uint       `toml:"interval"`
	Notifiers []notifier `toml:"notifiers"`
}

type notifier struct {
	Scheme   string          `toml:"type"`
	Location string          `toml:"location"`
	Source   string          `toml:"source"`
	Instance int32           `toml:"instance"`
	Channels []panda.Channel `toml:"channels"`
}

type decodeFunc func(io.Reader, []uint8) <-chan *hadock.Packet

func runListen(cmd *cli.Command, args []string) error {
	if err := cmd.Flag.Parse(args); err != nil {
		return err
	}
	f, err := os.Open(cmd.Flag.Arg(0))
	if err != nil {
		return err
	}
	c := struct {
		Addr      string   `toml:"address"`
		Mode      string   `toml:"mode"`
		Buffer    uint     `toml:"buffer"`
		Proxy     proxy    `toml:"proxy"`
		Instances []uint8  `toml:"instances"`
		Age       uint     `toml:"age"`
		Stores    []storer `toml:"storage"`
		Pool      pool     `toml:"pool"`
		Modules   []module `toml:"module"`
	}{}
	if err := toml.NewDecoder(f).Decode(&c); err != nil {
		return err
	} else {
		f.Close()
	}
	fs, err := setupStorage(c.Stores)
	if err != nil {
		return err
	}
	pool, err := setupPool(c.Pool)
	if err != nil {
		return err
	}
	ms, err := setupModules(c.Modules)
	if err != nil {
		return err
	}
	var df decodeFunc
	switch c.Mode {
	case "rfc1952", "gzip":
		df = hadock.DecodeCompressedPackets
	case "binary", "":
		df = hadock.DecodeBinaryPackets
	case "binary+gzip":
		df = func(r io.Reader, is []uint8) <-chan *hadock.Packet {
			if _, ok := r.(io.ByteReader); ok {
				r = bufio.NewReader(r)
			}
			if rs, err := gzip.NewReader(r); err == nil {
				//defer rs.Close()
				r = rs
			}
			return hadock.DecodeBinaryPackets(r, is)
		}
	default:
		return fmt.Errorf("unsupported working mode %s", c.Mode)
	}
	ps, err := ListenPackets(c.Addr, int(c.Buffer), c.Proxy, df, c.Instances)
	if err != nil {
		return err
	}
	queue := make(chan *hadock.Item, 1024)
	defer close(queue)
	go func() {
		for i := range queue {
			if err := ms.Process(uint8(i.Instance), i.HRPacket); err != nil {
				log.Printf("plugin: %s", err)
			}
		}
	}()

	age := time.Second * time.Duration(c.Age)
	for i := range Convert(ps) {
		if err := fs.Store(uint8(i.Instance), i.HRPacket); err != nil {
			log.Printf("storing VMU packet %s failed: %s", i.HRPacket.Filename(), err)
		}
		if age == 0 || time.Since(i.Timestamp()) <= age {
			pool.Notify(i)
		}
		select {
		case queue <- i:
		default:
		}
	}
	return nil
}

func Convert(ps <-chan *hadock.Packet) <-chan *hadock.Item {
	q := make(chan *hadock.Item, 1024)
	go func() {
		ds := make(map[int]panda.Decoder)
		for _, v := range []int{panda.VMUProtocol1, panda.VMUProtocol2} {
			d, err := panda.DecodeHR(v)
			if err != nil {
				continue
			}
			ds[v] = d
		}

		defer close(q)
		tick := time.Tick(time.Second)
		var (
			image   int
			science int
			dropped int
			errors  int
			size    int
		)
		logger := log.New(os.Stderr, "[hdk] ", 0)
		for p := range ps {
			d, ok := ds[int(p.Version)]
			if !ok {
				errors++
				log.Printf("no decoder available for version %d", p.Version)
				continue
			}
			_, v, err := d.Decode(p.Payload)
			if err != nil {
				errors++
				log.Printf("decoding VMU packet failed: %s", err)
				continue
			}
			var hr panda.HRPacket
			switch v.(type) {
			case *panda.Table:
				science++
				hr = v.(panda.HRPacket)
			case *panda.Image:
				image++
				hr = v.(panda.HRPacket)
			default:
				dropped++
				log.Println("unknown packet type - skipping")
				continue
			}
			select {
			case q <- &hadock.Item{int32(p.Instance), hr}:
				size += len(p.Payload)
			default:
				dropped++
			}
			select {
			case <-tick:
				logger.Printf("images: %6d, sciences: %6d, dropped: %6d, errors: %6d, size: %7dKB", image, science, dropped, errors, size>>10)
				size, image, science, dropped, errors = 0, 0, 0, 0, 0
			default:
			}

		}
	}()
	return q
}

func ListenPackets(a string, size int, p proxy, decode decodeFunc, is []uint8) (<-chan *hadock.Packet, error) {
	s, err := net.Listen("tcp", a)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		size++
	}
	q := make(chan *hadock.Packet, size)
	go func() {
		defer func() {
			s.Close()
			close(q)
		}()
		for {
			c, err := s.Accept()
			if err != nil {
				return
			}
			if c, ok := c.(*net.TCPConn); ok {
				c.SetKeepAlive(true)
				c.SetKeepAlivePeriod(time.Second * 90)
			}
			go func(c net.Conn) {
				defer c.Close()

				var r io.Reader = c
				if c, err := hadock.DialProxy(p.Addr, p.Level); err == nil {
					defer c.Close()
					r = io.TeeReader(r, c)
				}
				for p := range decode(r, is) {
					q <- p
				}
				log.Printf("connection closed: %s", c.RemoteAddr())
			}(c)
		}
	}()
	return q, nil
}

func setupModules(ms []module) (hadock.Module, error) {
	var ps []hadock.Module
	for _, m := range ms {
		p, err := plugin.Open(m.Location)
		if err != nil {
			return nil, err
		}
		n, err := p.Lookup("New")
		if err != nil {
			return nil, err
		}
		switch n := n.(type) {
		case func(string) (hadock.Module, error):
			for _, c := range m.Config {
				i, err := n(c)
				if err != nil {
					continue
				}
				ps = append(ps, i)
			}
		case func() (hadock.Module, error):
			i, err := n()
			if err != nil {
				continue
			}
			ps = append(ps, i)
		default:
			return nil, fmt.Errorf("invalid module function: %T", n)
		}
	}
	return hadock.Process(ps), nil
}

func setupPool(p pool) (*hadock.Pool, error) {
	delay := time.Second * time.Duration(p.Interval)

	ns := make([]hadock.Notifier, 0, len(p.Notifiers))
	for _, v := range p.Notifiers {
		var (
			err error
			n   hadock.Notifier
		)
		o := &hadock.Options{
			Source:   v.Source,
			Instance: v.Instance,
			Channels: v.Channels,
		}
		switch v.Scheme {
		default:
			continue
		case "udp":
			n, err = hadock.NewExternalNotifier(v.Scheme, v.Location, o)
		case "logger":
			var w io.Writer
			switch v.Location {
			default:
				f, e := os.Create(v.Location)
				if e != nil {
					return nil, err
				}
				w = f
			case "/dev/null":
				w = ioutil.Discard
			case "":
				w = os.Stdout
			}
			n, err = hadock.NewDebuggerNotifier(w, o)
		}
		if err != nil {
			return nil, err
		}
		ns = append(ns, n)
	}
	return hadock.NewPool(ns, delay), nil
}

func setupStorage(vs []storer) (hadock.Storage, error) {
	if len(vs) == 0 {
		return nil, fmt.Errorf("no storage defined! abort")
	}
	fs := make([]hadock.Storage, 0, len(vs))
	for _, v := range vs {
		if v.Disabled {
			continue
		}
		var (
			err error
			s   hadock.Storage
		)
		switch v.Scheme {
		default:
			continue
		case "file":
			if err = os.MkdirAll(v.Location, 0755); err != nil {
				log.Println("storage, fail to create primary directory: %s => %s", v.Location, err)
				break
			}
			if err = os.MkdirAll(v.Hard, 0755); v.Hard != "" && err != nil {
				log.Println("storage, fail to create secondary directory: %s => %s", v.Hard, err)
				break
			}
			s, err = hadock.NewLocalStorage(v.Location, v.Hard, int(v.Granularity), v.Raw, v.Remove)
		case "http":
			s, err = hadock.NewHTTPStorage(v.Location, int(v.Granularity))
		case "hrdp":
			if err = os.MkdirAll(v.Location, 0755); err != nil {
				break
			}
			s, err = hadock.NewHRDPStorage(v.Location, 2)
		}
		if err != nil {
			return nil, err
		}
		fs = append(fs, s)
	}
	return hadock.Multistore(fs...), nil
}
