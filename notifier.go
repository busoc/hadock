package hadock

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"io"
	"log"
	"net"
	"sort"
	"time"

	"github.com/busoc/panda"
)

const magic uint32 = 0x00

type Notifier interface {
	Accept(Message) error
	Notify(Message) error
}

type Item struct {
	Instance int32
	panda.HRPacket
}

type Message struct {
	Origin    string        `json:"origin"`
	Sequence  uint32        `json:"sequence"`
	Instance  int32         `json:"instance"`
	Channel   panda.Channel `json:"channel"`
	Realtime  bool          `json:"realtime"`
	Count     uint32        `json:"count"`
	Elapsed   time.Duration `json:"elapsed"`
	Generated int64         `json:"generated"`
	Acquired  int64         `json:"acquired"`
	Size      int64         `json:"size"`
	Bad       int64         `json:"bad"`
	Reference string        `json:"reference"`
	UPI       string        `json:"upi"`
}

func DecodeMessage(r io.Reader) (Message, error) {
	readString := func() (string, error) {
		var z uint16
		if err := binary.Read(r, binary.BigEndian, &z); err != nil {
			return "", err
		}
		bs := make([]byte, int(z))
		if _, err := io.ReadFull(r, bs); err != nil {
			return "", err
		}
		return string(bs), nil
	}
	var (
		msg Message
		err error
	)

	msg.Origin, err = readString()
	if err != nil {
		return msg, err
	}
	binary.Read(r, binary.BigEndian, &msg.Sequence)
	binary.Read(r, binary.BigEndian, &msg.Instance)
	binary.Read(r, binary.BigEndian, &msg.Channel)
	binary.Read(r, binary.BigEndian, &msg.Realtime)
	binary.Read(r, binary.BigEndian, &msg.Count)
	binary.Read(r, binary.BigEndian, &msg.Elapsed)
	binary.Read(r, binary.BigEndian, &msg.Generated) // VMU timestamp
	binary.Read(r, binary.BigEndian, &msg.Acquired)  // HRD timestamp
	msg.Reference, err = readString()
	if err != nil {
		return msg, err
	}
	msg.UPI, err = readString()
	if err != nil {
		return msg, err
	}

	return msg, nil
}

type Options struct {
	Source   string
	Instance int32
	Channels []panda.Channel
}

func (o *Options) Accept(msg Message) error {
	if o == nil {
		return nil
	}
	if o.Instance >= 0 && o.Instance != msg.Instance {
		return fmt.Errorf("instance %d not accepted", msg.Instance)
	}
	ix := sort.Search(len(o.Channels), func(i int) bool {
		return o.Channels[i] <= msg.Channel
	})
	if len(o.Channels) > 0 && (ix >= len(o.Channels) || o.Channels[ix] != msg.Channel) {
		return fmt.Errorf("channel %d not accepted", msg.Channel)
	}
	var ok bool
	switch o.Source {
	case "realtime":
		ok = msg.Realtime
	case "playback":
		ok = !msg.Realtime
	case "":
		ok = true
	}
	if !ok {
		return fmt.Errorf("source not accepted")
	}
	return nil
}

type Pool struct {
	notifiers []Notifier
	limit     time.Duration
	queue     chan *Item
}

func NewPool(ns []Notifier, a, e time.Duration) *Pool {
	ms := make([]Notifier, len(ns))
	copy(ms, ns)

	p := Pool{
		notifiers: ms,
		limit:     a,
	}
	if e > 0 {
		p.queue = make(chan *Item, 1000)
		go p.notify(e)
	}

	return &p
}

func (p *Pool) Notify(i *Item) {
	if p.queue == nil {
		return
	}
	var t time.Time
	switch v := i.HRPacket.(type) {
	case *panda.Image:
		t = v.VMUHeader.Timestamp()
	case *panda.Table:
		t = v.VMUHeader.Timestamp()
	default:
		return
	}
	if p.limit > 0 && time.Since(t) > p.limit {
		return
	}
	select {
	case p.queue <- i:
	default:
	}
}

func (p *Pool) notify(e time.Duration) {
	type key struct {
		Realtime bool
		Origin   string
		Instance int32
	}
	t := time.NewTicker(e)
	defer t.Stop()

	cache := make(map[key][]panda.HRPacket)
	for {
		select {
		case p, ok := <-p.queue:
			if !ok {
				return
			}
			k := key{p.IsRealtime(), p.Origin(), p.Instance}
			cache[k] = append(cache[k], p.HRPacket)
		case <-t.C:
			for k, ps := range cache {
				if len(ps) == 0 {
					continue
				}
				go func(k key, ps []panda.HRPacket) {
					sort.Slice(ps, func(i, j int) bool {
						return ps[i].Sequence() < ps[j].Sequence()
					})
					first, last := ps[0], ps[len(ps)-1]
					g := first.Timestamp()
					if v, ok := first.(interface {
						Generated() time.Time
					}); ok {
						g = v.Generated()
					}
					m := Message{
						Origin:    k.Origin,
						Instance:  int32(k.Instance),
						Realtime:  k.Realtime,
						Count:     uint32(len(ps)),
						Sequence:  first.Sequence(),
						Channel:   first.Stream(),
						Elapsed:   last.Timestamp().Sub(first.Timestamp()),
						Generated: g.Unix(),
						Acquired:  first.Timestamp().Unix(),
						Reference: first.Filename(),
						UPI:       extractUserInfo(first),
					}
					m.Size, m.Bad = sizeAndBad(ps)
					for _, n := range p.notifiers {
						go n.Notify(m)
					}
				}(k, ps)
				delete(cache, k)
			}
		}
	}
}

func sizeAndBad(ps []panda.HRPacket) (int64, int64) {
	var (
		size int64
		bad  int64
	)
	for _, p := range ps {
		if !panda.Valid(p) {
			bad++
		}
		size += int64(len(p.Payload()))
	}
	return size, bad
}

func extractUserInfo(p panda.HRPacket) string {
	var (
		bs  [32]byte
		alt string
	)
	switch v := p.(type) {
	case *panda.Table:
		alt = "SCIENCE"
		if p.IsRealtime() {
			return alt
		}
		s, ok := v.SDH.(*panda.SDHv2)
		if !ok {
			break
		}
		bs = s.Info
	case *panda.Image:
		alt = "IMAGE"
		if p.IsRealtime() {
			return alt
		}
		switch v := v.IDH.(type) {
		case *panda.IDHv2:
			bs = v.Info
		case *panda.IDHv1:
			bs = v.Info
		}
	}
	if upi := bytes.Trim(bs[:], "\x00"); len(upi) > 0 {
		return string(upi)
	}
	return alt
}

func NewDebuggerNotifier(w io.Writer, o *Options) (Notifier, error) {
	g := log.New(w, "[debug] ", log.LstdFlags)
	return &debugger{Logger: g, Options: o}, nil
}

func NewExternalNotifier(p, a string, o *Options) (Notifier, error) {
	c, err := net.Dial(p, a)
	if err != nil {
		return nil, err
	}
	return &notifier{conn: c, Options: o}, nil
}

type debugger struct {
	*Options
	*log.Logger
}

func (d *debugger) Notify(msg Message) error {
	if err := d.Accept(msg); err != nil {
		return nil
	}
	rate := float64(msg.Count)
	if secs := msg.Elapsed.Seconds(); secs > 0 {
		rate = float64(msg.Count) / secs
	}
	d.Logger.Printf("| %3d | %6s | %6d | %3d | %6d | %16s | %6.3f | %s | %s | %32s | %s",
		msg.Instance,
		msg.Origin,
		msg.Sequence,
		msg.Channel,
		msg.Count,
		msg.Elapsed,
		rate,
		panda.AdjustGenerationTime(msg.Generated).Format(time.RFC3339),
		panda.UNIX.Add(time.Duration(msg.Acquired)*time.Second).Format(time.RFC3339),
		msg.UPI,
		msg.Reference,
	)
	return nil
}

type notifier struct {
	*Options
	conn net.Conn
}

func (n *notifier) Notify(m Message) error {
	if err := n.Accept(m); err != nil {
		return nil
	}
	var (
		bs  []byte
		buf bytes.Buffer
	)

	bs = []byte(m.Origin)
	binary.Write(&buf, binary.BigEndian, uint16(len(bs)))
	buf.Write(bs)
	binary.Write(&buf, binary.BigEndian, m.Sequence)
	binary.Write(&buf, binary.BigEndian, m.Instance)
	binary.Write(&buf, binary.BigEndian, m.Channel)
	binary.Write(&buf, binary.BigEndian, m.Realtime)
	binary.Write(&buf, binary.BigEndian, m.Count)
	binary.Write(&buf, binary.BigEndian, m.Elapsed)
	binary.Write(&buf, binary.BigEndian, m.Generated)
	binary.Write(&buf, binary.BigEndian, m.Acquired)
	binary.Write(&buf, binary.BigEndian, m.Size)
	binary.Write(&buf, binary.BigEndian, m.Bad)
	bs = []byte(m.Reference)
	binary.Write(&buf, binary.BigEndian, uint16(len(bs)))
	buf.Write(bs)
	bs = []byte(m.UPI)
	binary.Write(&buf, binary.BigEndian, uint16(len(bs)))
	buf.Write(bs)

	_, err := io.Copy(n.conn, &buf)
	return err
}
