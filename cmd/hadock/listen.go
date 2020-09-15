package main

import (
	"bufio"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"plugin"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/hadock/cascading"
	"github.com/busoc/hadock/storage"
	"github.com/busoc/panda"
	"github.com/midbel/cli"
	"github.com/midbel/toml"
)

func init() {
	now := time.Now()
	uptime := expvar.Func(func() interface{} {
		return time.Since(now).Seconds()
	})
	expvar.Publish("uptime", uptime)
}

type proxy struct {
	Addr string `toml:"address"`
	Size int    `toml:"size"`
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
		Addr      string            `toml:"address"`
		Mon       string            `toml:"monitor"`
		Mode      string            `toml:"mode"`
		Buffer    uint              `toml:"buffer"`
		Proxy     proxy             `toml:"proxy"`
		Instances []uint8           `toml:"instances"`
		Stores    []storage.Options `toml:"storage"`
		Pool      pool              `toml:"pool"`
		Modules   []module          `toml:"module"`
	}{}
	if err := toml.Decode(f, &c); err != nil {
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

	if c.Mon != "" {
		go func() {
			if err := http.ListenAndServe(c.Mon, nil); err != nil {
				log.Printf("monitor: fail to listen on %s: %s", c.Mon, err)
			}
		}()
	}

	df, err := Decode(c.Mode)
	if err != nil {
		return err
	}

	ps, err := ListenPackets(c.Addr, int(c.Buffer), c.Proxy, df, c.Instances)
	if err != nil {
		return err
	}
	var queue chan *hadock.Item
	if len(c.Modules) > 0 {
		ms, err := setupModules(c.Modules)
		if err != nil {
			return err
		}
		queue = make(chan *hadock.Item, int(c.Buffer))
		defer close(queue)
		go func() {
			logger := log.New(os.Stderr, "[plugin] ", 0)
			for i := range queue {
				if err := ms.Process(uint8(i.Instance), i.HRPacket); err != nil {
					logger.Println(err)
				}
			}
		}()
	}

	for i := range Convert(ps, int(c.Buffer)) {
		if err := fs.Store(uint8(i.Instance), i.HRPacket); err != nil {
			log.Printf("storing VMU packet %s failed: %s", i.HRPacket.Filename(), err)
		}
		pool.Notify(i)
		select {
		case queue <- i:
		default:
		}
	}
	return nil
}

func Decode(mode string) (decodeFunc, error) {
	var df decodeFunc
	switch mode {
	case "rfc1952", "gzip":
		df = hadock.DecodeCompressedPackets
	case "binary", "":
		df = hadock.DecodeBinaryPackets
	default:
		return nil, fmt.Errorf("unsupported working mode %s", mode)
	}
	return df, nil
}

func Convert(ps <-chan *hadock.Packet, n int) <-chan *hadock.Item {
	q := make(chan *hadock.Item, n)

	var (
		total   = expvar.NewInt("total")   //int64
		image   = expvar.NewInt("image")   //int64
		science = expvar.NewInt("science") //int64
		skipped = expvar.NewInt("skipped") //int64
		errors  = expvar.NewInt("errors")  //int64
		size    = expvar.NewInt("size")    //int64
		wait    = expvar.NewFloat("wait")
	)

	go func() {
		logger := log.New(os.Stderr, "[hdk] ", 0)
		var (
			tick = time.Tick(time.Second)
			prev time.Time
		)

		for t := range tick {
			if total.Value() > 0 || skipped.Value() > 0 || errors.Value() > 0 {
				logger.Printf("%6d total, %6d images, %6d sciences, %6d skipped, %6d errors, %7dKB",
					total.Value(),
					image.Value(),
					science.Value(),
					skipped.Value(),
					errors.Value(),
					size.Value()>>10)
				size.Set(0)
				image.Set(0)
				science.Set(0)
				skipped.Set(0)
				errors.Set(0)
				total.Set(0)
				wait.Set(0)
			} else {
				if !prev.IsZero() {
					elapsed := t.Sub(prev)
					wait.Add(float64(elapsed.Milliseconds()))
				}
				prev = t
			}
		}
	}()
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
		logger := log.New(os.Stderr, "[error] ", 0)
		for p := range ps {
			d, ok := ds[int(p.Version)]
			if !ok {
				errors.Add(1)
				// errors++
				logger.Printf("no decoder available for version %d", p.Version)
				continue
			}
			_, v, err := d.Decode(p.Payload)
			if err != nil {
				errors.Add(1)
				// errors++
				logger.Printf("decoding VMU packet failed: %s", err)
				continue
			}
			var hr panda.HRPacket
			switch v.(type) {
			case *panda.Table:
				science.Add(1)
				// science++
				hr = v.(panda.HRPacket)
			case *panda.Image:
				image.Add(1)
				// image++
				hr = v.(panda.HRPacket)
			default:
				errors.Add(1)
				// errors++
				logger.Println("unknown packet type - skipping")
				continue
			}
			select {
			case q <- &hadock.Item{int32(p.Instance), hr}:
				total.Add(1)
				// total++
				size.Add(int64(len(p.Payload)))
				// size += int64(len(p.Payload))
			default:
				skipped.Add(1)
				// skipped++
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
				if c, err := cascading.Proxy(p.Addr, p.Size); err == nil {
					defer c.Close()
					r = io.TeeReader(r, c)
				}
				rs := bufio.NewReaderSize(r, 32<<20)
				for p := range decode(rs, is) {
					q <- p
				}
			}(c)
		}
	}()
	return q, nil
}

type module struct {
	Location string   `toml:"location"`
	Config   []string `toml:"config"`
}

func setupModules(ms []module) (hadock.Module, error) {
	if len(ms) == 0 {
		return nil, nil
	}
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

type pool struct {
	Interval  uint       `toml:"interval"`
	Limit     uint       `toml:"age"`
	Notifiers []notifier `toml:"notifiers"`
}

type notifier struct {
	Scheme   string          `toml:"type"`
	Location string          `toml:"location"`
	Source   string          `toml:"source"`
	Instance int32           `toml:"instance"`
	Channels []panda.Channel `toml:"channels"`
}

func setupPool(p pool) (*hadock.Pool, error) {
	delay := time.Second * time.Duration(p.Interval)
	age := time.Second * time.Duration(p.Limit)

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
	return hadock.NewPool(ns, age, delay), nil
}

func setupStorage(vs []storage.Options) (storage.Storage, error) {
	if len(vs) == 0 {
		return nil, fmt.Errorf("no storage defined! abort")
	}
	fs := make([]storage.Storage, 0, len(vs))
	for _, v := range vs {
		var (
			err error
			s   storage.Storage
		)
		switch v.Scheme {
		default:
			err = fmt.Errorf("%s: unrecognized storage type", v.Scheme)
		case "":
			continue
		case "hrdp":
			if err = mkdirAll(v); err != nil {
				log.Printf("storage: fail to create directories of %s: %v", v.Scheme, err)
				break
			}
			s, err = storage.NewHRDPStorage(v)
		case "tar", "archive":
			if err = mkdirAll(v); err != nil {
				log.Printf("storage: fail to create directories of %s: %v", v.Scheme, err)
				break
			}
			s, err = storage.NewArchiveStorage(v)
		case "file":
			if err = mkdirAll(v); err != nil {
				log.Printf("storage: fail to create directories of %s: %v", v.Scheme, err)
				break
			}
			s, err = storage.NewLocalStorage(v)
		}
		if err != nil {
			return nil, err
		}
		fs = append(fs, s)
	}
	return storage.Multistore(fs...), nil
}

func mkdirAll(v storage.Options) error {
	if err := os.MkdirAll(v.Location, 0755); v.Location != "" && err != nil {
		return err
	}
	for _, s := range v.Shares {
		if err := os.MkdirAll(s.Location, 0755); s.Location != "" && err != nil {
			return err
		}
	}
	return nil
}
