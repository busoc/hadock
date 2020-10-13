package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/hadock/cmd/hdk2udp/internal/pvalue"
	"github.com/busoc/hadock/cmd/hdk2udp/internal/yamcs"
	"github.com/busoc/hadock/storage"
	"github.com/busoc/panda"
	"github.com/golang/protobuf/proto"
	"github.com/midbel/toml"
	"golang.org/x/sync/errgroup"
)

func main() {
	flag.Parse()

	c := struct {
		Channels []channel `toml:"channel"`
	}{}
	if err := toml.DecodeFile(flag.Arg(0), &c); err != nil {
		log.Fatalln(err)
	}
	var grp errgroup.Group
	for _, c := range c.Channels {
		r := c
		grp.Go(func() error {
			if len(r.Levels) == 0 {
				r.Levels = []string{storage.LevelClassic, storage.LevelVMUTime}
			}
			defer log.Printf("done sending packets to %s", r.Link)
			log.Printf("start sending packets to %s", r.Link)

			return r.Run()
		})
	}
	if err := grp.Wait(); err != nil {
		log.Println(err)
	}
}

type conn struct {
	net.Conn
	addr string
}

func (c *conn) Write(bs []byte) (int, error) {
	n, err := c.Conn.Write(bs)
	if err == nil {
		return n, err
	}
	if err, ok := err.(net.Error); ok && !err.Temporary() {
		s, err := net.Dial("udp", c.addr)
		if err != nil {
			return len(bs), nil
		}
		c.Conn.Close()
		c.Conn = s
	}
	return len(bs), nil
}

type group struct {
	Addr string `toml:"addr"`
	Ifi  string `toml:"interface"`
}

type channel struct {
	Link   string  `toml:"address"`
	Name   string  `toml:"namespace"`
	Groups []group `toml:"groups"`

	Prefix   string   `toml:"prefix"`
	Time     string   `toml:"time"`
	Interval int      `toml:"interval"`
	Levels   []string `toml:"levels"`
}

func (c channel) Run() error {
	w, err := connect(c.Link)
	if err != nil {
		return err
	}
	q := make(chan hadock.Message, 100)
	defer close(q)
	go sendTo(q, w, c.Name)

	var wg sync.WaitGroup
	for _, g := range c.Groups {
		r, err := subscribe(g.Addr, g.Ifi)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func(rc net.Conn) {
			defer func() {
				rc.Close()
				wg.Done()
			}()
			//r := bufio.NewReader(rc)
			buf := make([]byte, 4096)
			for {
				n, err := rc.Read(buf)
				if err != nil {
					log.Printf("unexpected error while reading PP from %s: %s", rc.RemoteAddr(), err)
					continue
				}
				if n == 0 {
					continue
				}
				m, err := hadock.DecodeMessage(bytes.NewBuffer(buf[:n]))
				if err != nil {
					log.Println("decoding hadock message failed:", err)
					continue
				}
				m.Reference = c.Prepare(m)
				log.Println(m.Reference)
				q <- m
			}
		}(r)
	}
	wg.Wait()
	return nil
}

func (c channel) Prepare(m hadock.Message) string {
	var t time.Time
	switch strings.ToLower(c.Time) {
	case "acq":
		t = time.Unix(m.Acquired, 0)
	default:
		when := panda.GenerationTimeFromEpoch(m.Generated) / 1000
		t = time.Unix(when, 0)
	}
	base := prepareReference(c.Prefix, c.Levels, m, c.Interval, t)
	ref := path.Join(base, m.Reference)
	if !strings.HasPrefix(ref, "/") {
		ref = "/" + ref
	}
	return ref
}

func sendTo(queue <-chan hadock.Message, w net.Conn, n string) {
	defer w.Close()

	var i uint16
	for m := range queue {
		i++
		bs, err := marshal(m, n, int32(i))
		if err != nil {
			log.Println("fail to prepare PP:", err)
			continue
		}
		if _, err := w.Write(bs); err != nil {
			log.Printf("fail to sendd PP to %s: %s", w.RemoteAddr(), err)
		}
	}
}

func prepareReference(base string, levels []string, m hadock.Message, g int, t time.Time) string {
	for _, n := range levels {
		switch strings.ToLower(n) {
		default:
			base = path.Join(base, n)
		case storage.LevelUPI:
			base = path.Join(base, m.UPI)
		case storage.LevelClassic:
			ns := []string{storage.LevelInstance, storage.LevelType, storage.LevelMode, storage.LevelSource}
			base = prepareReference(base, ns, m, g, t)
		case storage.LevelSource:
			base = path.Join(base, m.Origin)
		case storage.LevelInstance:
			base = whichInstance(base, m)
		case storage.LevelType:
			base = whichType(base, m)
		case storage.LevelMode:
			base = whichMode(base, m)
		case storage.LevelVMUTime:
			when := panda.GenerationTimeFromEpoch(m.Generated)
			ns := []string{storage.LevelYear, storage.LevelDay, storage.LevelHour, storage.LevelMin}
			base = prepareReference(base, ns, m, g, time.Unix(when/1000, 0))
		case storage.LevelACQTime:
			ns := []string{storage.LevelYear, storage.LevelDay, storage.LevelHour, storage.LevelMin}
			base = prepareReference(base, ns, m, g, time.Unix(m.Acquired, 0))
		case storage.LevelYear:
			base = path.Join(base, fmt.Sprintf("%04d", t.Year()))
		case storage.LevelDay:
			base = path.Join(base, fmt.Sprintf("%03d", t.YearDay()))
		case storage.LevelHour:
			base = path.Join(base, fmt.Sprintf("%02d", t.Hour()))
		case storage.LevelMin:
			if m := t.Truncate(time.Second * time.Duration(g)); g > 0 {
				base = path.Join(base, fmt.Sprintf("%02d", m.Minute()))
			}
		}
	}
	return base
}

func whichInstance(base string, m hadock.Message) string {
	switch m.Instance {
	case hadock.TEST:
		base = path.Join(base, "TEST")
	case hadock.SIM1, hadock.SIM2:
		base = path.Join(base, "SIM"+fmt.Sprint(m.Instance))
	case hadock.OPS:
		base = path.Join(base, "OPS")
	default:
		base = path.Join(base, "DATA-"+fmt.Sprint(m.Instance))
	}
	return base
}

func whichMode(base string, m hadock.Message) string {
	if m.Realtime {
		base = path.Join(base, "realtime")
	} else {
		base = path.Join(base, "playback")
	}
	return base
}

func whichType(base string, m hadock.Message) string {
	switch m.Channel {
	case 1, 2:
		base = path.Join(base, "images")
	case 3:
		base = path.Join(base, "sciences")
	default:
		base = path.Join(base, "unknown")
	}
	return base
}

type value struct {
	Local string
	Name  string
	Value interface{}
}

func marshal(m hadock.Message, n string, i int32) ([]byte, error) {
	w, t := time.Now().UTC().Unix(), m.Generated
	adjw := panda.AcquisitionTimeFromEpoch(w)
	adjt := panda.GenerationTimeFromEpoch(t)

	pd := &pvalue.ParameterData{
		Group:          &n,
		GenerationTime: &adjt,
		SeqNum:         &i,
	}
	vs := []value{
		{Local: n, Name: "origin", Value: m.Origin},
		{Local: n, Name: "sequence", Value: m.Sequence},
		{Local: n, Name: "instance", Value: m.Instance},
		{Local: n, Name: "channel", Value: m.Channel},
		{Local: n, Name: "realtime", Value: m.Realtime},
		{Local: n, Name: "count", Value: m.Count},
		{Local: n, Name: "elapsed", Value: m.Elapsed},
		{Local: n, Name: "timestamp", Value: adjt / 1000},
		{Local: n, Name: "reference", Value: m.Reference},
		{Local: n, Name: "upi", Value: m.UPI},
	}

	for _, v := range vs {
		v.Name = fmt.Sprintf("%v_%v", m.Origin, v.Name)
		pd.Parameter = append(pd.Parameter, marshalParameter(v, adjw, adjt))
	}
	return proto.Marshal(pd)
}

//w = acquisition (Hadock - now), t = generation (VMU/HRD)
func marshalParameter(v value, w, t int64) *pvalue.ParameterValue {
	var x yamcs.Value

	switch v := v.Value.(type) {
	case string:
		t := yamcs.Value_STRING
		x = yamcs.Value{
			Type:        &t,
			StringValue: &v,
		}
	case uint32:
		t := yamcs.Value_UINT32
		x = yamcs.Value{
			Type:        &t,
			Uint32Value: &v,
		}
	case panda.Channel:
		t := yamcs.Value_SINT32
		d := int32(v)
		x = yamcs.Value{
			Type:        &t,
			Sint32Value: &d,
		}
	case int32:
		t := yamcs.Value_SINT32
		x = yamcs.Value{
			Type:        &t,
			Sint32Value: &v,
		}
	case time.Duration:
		t := yamcs.Value_SINT64
		d := int64(v)
		x = yamcs.Value{
			Type:        &t,
			Sint64Value: &d,
		}
	case int64:
		t := yamcs.Value_SINT64
		x = yamcs.Value{
			Type:        &t,
			Sint64Value: &v,
		}
	case bool:
		t := yamcs.Value_BOOLEAN
		x = yamcs.Value{
			Type:         &t,
			BooleanValue: &v,
		}
	}
	status := pvalue.AcquisitionStatus_ACQUIRED
	name := fmt.Sprintf("%s%s", v.Local, v.Name)
	uid := yamcs.NamedObjectId{
		Name:      &name,
		Namespace: &v.Local,
	}
	return &pvalue.ParameterValue{
		Id:                &uid,
		EngValue:          &x,
		AcquisitionStatus: &status,
		GenerationTime:    &t,
		AcquisitionTime:   &w,
	}
}

func connect(a string) (net.Conn, error) {
	c, err := net.Dial("udp", a)
	if err != nil {
		return nil, err
	}
	return &conn{c, a}, nil
}

func subscribe(a, i string) (net.Conn, error) {
	addr, err := net.ResolveUDPAddr("udp", a)
	if err != nil {
		return nil, err
	}
	var ifi *net.Interface
	if i, err := net.InterfaceByName(i); err == nil {
		ifi = i
	}
	return net.ListenMulticastUDP("udp", ifi, addr)
}
