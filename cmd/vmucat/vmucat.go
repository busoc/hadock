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

type source string

func (s *source) String() string {
	return string(*s)
}

func (s *source) Set(v string) error {
	switch v {
	case "realtime", "playback", "", "*":
		*s = source(v)
	default:
		return fmt.Errorf("unknown source %q", v)
	}
	return nil
}

const pattern = "2006-01-02T15:04:05.000Z"

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	var (
		cs  channels
		src source
		vmu int
	)
	flag.Var(&cs, "c", "channels")
	flag.Var(&src, "s", "source")
	flag.IntVar(&vmu, "u", panda.VMUProtocol2, "vmu version")
	flag.Parse()

	queue, err := Packets(flag.Arg(0), string(src), vmu, []panda.Channel(cs))
	if err != nil {
		log.Fatalln(err)
	}
	for p := range queue {
		var v *panda.VMUHeader

		acquisition, auxiliary, upi := "-", "-", "-"
		switch p := p.(type) {
		case *panda.Table:
			v = p.VMUHeader
			if a, ok := p.SDH.(Acquirer); ok {
				x, s := a.Acquire()
				auxiliary, acquisition = x.Format(pattern), s.Format(pattern)
			}
		case *panda.Image:
			v = p.VMUHeader
			if a, ok := p.IDH.(Acquirer); ok {
				x, s := a.Acquire()
				auxiliary, acquisition = x.Format(pattern), s.Format(pattern)
			}
			switch v := p.IDH.(type) {
			case *panda.IDHv1:
				upi = string(bytes.Trim(v.Info[:], "\x00"))
			case *panda.IDHv2:
				upi = string(bytes.Trim(v.Info[:], "\x00"))
			}
		default:
			continue
		}

		log.Printf("%3d | %s | %4s | %6t | %6d | %6d | %s | %7d | %-48s | %-6s | %s | %s | %s",
			p.Version(),
			v.Timestamp().Format(pattern),
			v.Stream(),
			p.IsRealtime(),
			v.Sequence,
			p.Sequence(),
			p.Origin(),
			len(p.Payload()),
			p.Filename(),
			p.Format(),
			auxiliary,
			acquisition,
			upi,
		)
	}
}

func Packets(a, s string, v int, cs []panda.Channel) (<-chan panda.HRPacket, error) {
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
			ix := sort.Search(len(cs), func(i int) bool {
				return cs[i] <= v.Stream()
			})
			if len(cs) > 0 && (ix >= len(cs) || cs[ix] != v.Stream()) {
				continue
			}
			ok = false
			switch r := v.IsRealtime(); s {
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
