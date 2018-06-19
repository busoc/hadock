package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/busoc/panda"
)

type Acquirer interface {
	Acquire() (time.Time, time.Time)
}

type channels []panda.Channel

func (cs *channels) String() string {
	return fmt.Sprint(*cs)
}

func (cs *channels) Set(vs string) error {
	for _, n := range strings.Split(vs, ",") {
		var c panda.Channel
		switch n {
		case "vic1":
			c = panda.Video1
		case "vic2":
			c = panda.Video2
		case "lrsd":
			c = panda.Science
		default:
			return fmt.Errorf("unknown channel %q", n)
		}
		*cs = append(*cs, c)
	}
	sort.SliceStable(*cs, func(i, j int) bool {
		return (*cs)[i] > (*cs)[j]
	})
	return nil
}

type mode string

func (m *mode) String() string {
	return string(*m)
}

func (m *mode) Set(v string) error {
	switch v {
	case "realtime", "playback", "", "*":
		*m = mode(v)
	default:
		return fmt.Errorf("unknown source %q", v)
	}
	return nil
}

const pattern = "2006-01-02T15:04:05.000Z"

type Coze struct {
	Count, Size uint64
}

type Counter struct {
	UPI     string
	Sources map[string]*Coze
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
}

func main() {
	var (
		cs  channels
		mod mode
	)
	flag.Var(&cs, "c", "channels")
	flag.Var(&mod, "m", "mode")
	list := flag.Bool("l", false, "list")
	info := flag.String("i", "", "upi")
	src := flag.String("s", "", "source")
	vmu := flag.Int("p", panda.VMUProtocol2, "vmu version")
	flag.Parse()

	queue, err := Packets(flag.Arg(0), string(mod), *src, *vmu, []panda.Channel(cs))
	if err != nil {
		log.Fatalln(err)
	}
	var count, size uint64
	counter := make(map[string]*Counter)
	for p := range queue {
		var v *panda.VMUHeader

		upi := "-"
		switch p := p.(type) {
		case *panda.Table:
			v = p.VMUHeader
			if v, ok := p.SDH.(*panda.SDHv2); ok {
				upi = string(bytes.Trim(v.Info[:], "\x00"))
			}
		case *panda.Image:
			v = p.VMUHeader
			switch v := p.IDH.(type) {
			case *panda.IDHv1:
				upi = string(bytes.Trim(v.Info[:], "\x00"))
			case *panda.IDHv2:
				upi = string(bytes.Trim(v.Info[:], "\x00"))
			}
		default:
			continue
		}
		if len(*info) > 0 && upi != *info {
			continue
		}
		valid := "-"
		if !panda.Valid(p) {
			valid = "corrupted"
		}
		c, ok := counter[upi]
		if !ok {
			c = &Counter{UPI: upi, Sources: make(map[string]*Coze)}
		}
		if z, ok := c.Sources[p.Origin()]; !ok {
			z = &Coze{Count: 1, Size: uint64(len(p.Payload()))}
			c.Sources[p.Origin()] = z
		} else {
			c.Sources[p.Origin()].Size += uint64(len(p.Payload()))
			c.Sources[p.Origin()].Count++
		}
		counter[upi] = c

		if *list {
			log.Printf("%s | %s | %4s | %6t | %6d | %6d | %7d | %-56s | %-6s | %s | %16s | %s",
				p.Origin(),
				v.Timestamp().Format(pattern),
				v.Stream(),
				p.IsRealtime(),
				v.Sequence,
				p.Sequence(),
				len(p.Payload()),
				p.Filename(),
				p.Format(),
				p.Timestamp().Format(time.RFC3339),
				upi,
				valid,
			)
		}
		size += uint64(len(p.Payload()))
		count++
	}
	log.Printf("%d packets found (%dMB)", count, size>>20)
	if *list {
		return
	}
	for _, c := range counter {
		for s, z := range c.Sources {
			log.Printf("%4s: %32s: %12d -> %12dMB", s, c.UPI, z.Count, z.Size>>20)
		}
	}
}

func Packets(a, mode, src string, v int, cs []panda.Channel) (<-chan panda.HRPacket, error) {
	d, err := panda.DecodeHR(v)
	if err != nil {
		return nil, err
	}
	w, err := panda.Walk("hr", a)
	if err != nil {
		return nil, err
	}
	q := make(chan panda.HRPacket)
	go func() {
		r := panda.NewReader(w, d)
		defer func() {
			close(q)
			r.Close()
		}()
		for {
			p, err := r.Read()
			switch err {
			case panda.ErrDone:
				return
			case nil:
			default:
				log.Fatalln(err)
			}
			v, ok := p.(panda.HRPacket)
			if !ok {
				continue
			}
			if len(src) > 0 && v.Origin() != src {
				continue
			}
			ix := sort.Search(len(cs), func(i int) bool {
				return cs[i] <= v.Stream()
			})
			if len(cs) > 0 && (ix >= len(cs) || cs[ix] != v.Stream()) {
				continue
			}
			ok = false
			switch r := v.IsRealtime(); mode {
			case "realtime":
				ok = r
			case "playback":
				ok = !r
			default:
				ok = true
			}
			if !ok {
				continue
			}
			q <- v
		}
	}()
	return q, nil
}
