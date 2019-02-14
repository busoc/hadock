package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/panda"
)

type Control struct {
	Type   string   `toml:"type"`
	Accept []string `toml:"accept"`
	Reject []string `toml:"reject"`
}

func (c *Control) Can(p panda.HRPacket) bool {
	if c == nil {
		return true
	}
	var o string
	switch c.Type {
	case "", "channel":
		o = p.Stream().String()
	case "origin", "source":
		o = p.Origin()
	default:
		return false
	}
	if len(c.Accept) == 0 && len(c.Reject) == 0 {
		return true
	}
	if len(c.Accept) == 0 {
		return !checkOrigin(o, c.Reject)
	}
	return checkOrigin(o, c.Accept) || !checkOrigin(o, c.Reject)
}

func checkOrigin(o string, vs []string) bool {
	if len(vs) == 0 {
		return true
	}
	ix := sort.SearchStrings(vs, o)
	return ix < len(vs) && vs[ix] == o
}

type Options struct {
	Scheme   string `toml:"type"`
	Location string `toml:"location"`
	Format   string `toml:"format"`
	Compress bool   `toml:"compress"`
	KeepBad  bool   `toml:"keep-bad"`
	// Instances []uint8 `toml:"instances"`
	Timeout int `toml:"timeout"`

	Control `toml:"control"`

	Interval int        `toml:"interval"`
	Epoch    string     `toml:"time"`
	Levels   []string   `toml:"levels"`
	Shares   []*Options `toml:"share"`

	Link string `toml:"link"`
}

const (
	LevelClassic  = "classic" // instance+type+mode+source
	LevelUPI      = "upi"
	LevelSource   = "source"
	LevelInstance = "instance" // OPS, SIM1, SIM2, TEST
	LevelType     = "type"     // images, sciences
	LevelMode     = "mode"     // realtime, playback
	LevelYear     = "year"
	LevelDay      = "doy"
	LevelHour     = "hour"
	LevelMin      = "minute"
	LevelVMUTime  = "vmu" // vmu: year+doy+hour+min
	LevelACQTime  = "acq" // acq: year+doy+hour+min
)

type Storage interface {
	Store(uint8, panda.HRPacket) error
}

func Multistore(s ...Storage) Storage {
	if len(s) == 1 {
		return s[0]
	}
	ms := make([]Storage, len(s))
	copy(ms, s)
	return &multistore{ms}
}

type multistore struct {
	ms []Storage
}

func (m *multistore) Close() error {
	var err error
	for _, s := range m.ms {
		c, ok := s.(io.Closer)
		if ok {
			if e := c.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

func (m *multistore) Store(i uint8, p panda.HRPacket) error {
	var err error
	for _, s := range m.ms {
		if e := s.Store(i, p); e != nil {
			err = e
		}
	}
	return err
}

type dirmaker struct {
	Levels   []string `toml:"levels"`
	Base     string   `toml:"location"`
	Time     string   `toml:"time"`
	Interval int      `toml:"-"`

	mu    sync.Mutex
	cache map[string]time.Time
}

func (d *dirmaker) clean() {
	every := time.Tick(time.Minute)
	five := time.Minute * 5
	for t := range every {
		for k, v := range d.cache {
			if t.Sub(v) >= five {
				d.mu.Lock()
				delete(d.cache, k)
				d.mu.Unlock()
			}
		}
	}
}

func (d *dirmaker) Prepare(i uint8, p panda.HRPacket) (string, error) {
	var t time.Time
	switch strings.ToLower(d.Time) {
	case "vmu", "":
		t = getVMUTime(p)
	case "acq":
		t = getACQTime(p)
	default:
	}
	if t.IsZero() {
		t = p.Timestamp()
	}
	base := prepareDirectory(d.Base, d.Levels, d.Interval, i, p, t)
	if d.cache != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, ok := d.cache[base]; !ok {
			if err := os.MkdirAll(base, 0755); err != nil && !os.IsExist(err) {
				return "", err
			}
		}
		d.cache[base] = time.Now()
	}
	return base, nil
}

func checkLevels(ls, ds []string) []string {
	var vs []string
	for i := range ls {
		if ls[i] == "" {
			continue
		}
		vs = append(vs, ls[i])
	}
	if len(vs) == 0 {
		vs = ds
	}
	return vs
}

func prepareDirectory(base string, levels []string, g int, i uint8, p panda.HRPacket, t time.Time) string {
	for _, n := range levels {
		switch strings.ToLower(n) {
		default:
			base = path.Join(base, n)
		case LevelClassic:
			ns := []string{LevelInstance, LevelType, LevelMode, LevelSource}
			base = prepareDirectory(base, ns, g, i, p, t)
		case LevelUPI:
			base = path.Join(base, getUPI(p))
		case LevelInstance:
			base = instanceDir(base, i)
		case LevelType:
			base = typeDir(base, p)
		case LevelMode:
			base = modeDir(base, p)
		case LevelSource:
			base = path.Join(base, p.Origin())
		case LevelYear:
			base = path.Join(base, fmt.Sprintf("%04d", t.Year()))
		case LevelDay:
			base = path.Join(base, fmt.Sprintf("%03d", t.YearDay()))
		case LevelHour:
			base = path.Join(base, fmt.Sprintf("%02d", t.Hour()))
		case LevelMin:
			if m := t.Truncate(time.Second * time.Duration(g)); g > 0 {
				base = path.Join(base, fmt.Sprintf("%02d", m.Minute()))
			}
		case LevelVMUTime:
			ns := []string{LevelYear, LevelDay, LevelHour, LevelMin}
			base = prepareDirectory(base, ns, g, i, p, getVMUTime(p))
		case LevelACQTime:
			ns := []string{LevelYear, LevelDay, LevelHour, LevelMin}
			base = prepareDirectory(base, ns, g, i, p, getACQTime(p))
		}
	}
	return base
}

func getVMUTime(p panda.HRPacket) time.Time {
	var t time.Time
	switch p := p.(type) {
	case *panda.Table:
		t = p.VMUHeader.Timestamp()
	case *panda.Image:
		t = p.VMUHeader.Timestamp()
	}
	return panda.AdjustGenerationTime(t.Unix())
}

func getACQTime(p panda.HRPacket) time.Time {
	return p.Timestamp()
}

func getUPI(p panda.HRPacket) string {
	trim := func(bs []byte) string {
		if bs := bytes.Trim(bs, "\x00"); len(bs) > 0 {
			return strings.Replace(string(bs), " ", "-", -1)
		}
		return ""
	}
	var upi, u string
	switch p := p.(type) {
	case *panda.Table:
		upi = "SCIENCES"
		switch v := p.SDH.(type) {
		case *panda.SDHv1:
		case *panda.SDHv2:
			u = trim(v.Info[:])
		}
	case *panda.Image:
		upi = "IMAGES"
		switch v := p.IDH.(type) {
		case *panda.IDHv1:
			u = trim(v.Info[:])
		case *panda.IDHv2:
			u = trim(v.Info[:])
		}
	}
	if len(u) > 0 {
		upi = u
	}
	return upi
}

func modeDir(base string, p panda.HRPacket) string {
	if p.IsRealtime() {
		base = path.Join(base, "realtime")
	} else {
		base = path.Join(base, "playback")
	}
	return base
}

func typeDir(base string, p panda.HRPacket) string {
	switch p.(type) {
	case *panda.Table:
		base = path.Join(base, "sciences")
	case *panda.Image:
		base = path.Join(base, "images")
	default:
		base = path.Join(base, "unknown")
	}
	return base
}

func instanceDir(base string, i uint8) string {
	switch i {
	case hadock.TEST:
		base = path.Join(base, "TEST")
	case hadock.SIM1, hadock.SIM2:
		base = path.Join(base, "SIM"+fmt.Sprint(i))
	case hadock.OPS:
		base = path.Join(base, "OPS")
	default:
		base = path.Join(base, "DATA-"+fmt.Sprint(i))
	}
	return base
}

func encodeMetadata(w io.Writer, p *panda.Image) error {
	m := struct {
		XMLName xml.Name  `xml:"metadata"`
		Version int       `xml:"mark,attr"`
		When    time.Time `xml:"vmu,attr"`
		IDH     interface{}
	}{
		Version: p.Version(),
		When:    p.VMUHeader.Timestamp(),
		IDH:     p.IDH,
	}
	e := xml.NewEncoder(w)
	e.Indent("", "\t")
	return e.Encode(m)
}

func encodeRawPacket(w io.Writer, p panda.HRPacket) error {
	var err error
	switch p := p.(type) {
	case *panda.Table:
		i, ok := p.SDH.(panda.Four)
		if !ok {
			err = p.ExportRaw(w)
			break
		}
		r := new(bytes.Buffer)
		binary.Write(r, binary.BigEndian, i.FCC())
		binary.Write(r, binary.BigEndian, p.Sequence())
		if s, ok := p.SDH.(*panda.SDHv2); ok {
			binary.Write(r, binary.BigEndian, s.Acquisition)
		} else {
			binary.Write(r, binary.BigEndian, p.Timestamp().Unix())
		}
		r.Write(p.Payload())

		_, err = io.Copy(w, r)
	case *panda.Image:
		i, ok := p.IDH.(panda.Bitmap)
		if !ok {
			err = p.ExportRaw(w)
			break
		}
		r := new(bytes.Buffer)

		binary.Write(r, binary.BigEndian, i.FCC())
		binary.Write(r, binary.BigEndian, p.Sequence())
		if i, ok := p.IDH.(*panda.IDHv2); ok {
			binary.Write(r, binary.BigEndian, i.Acquisition)
		} else {
			binary.Write(r, binary.BigEndian, p.Timestamp().Unix())
		}
		binary.Write(r, binary.BigEndian, i.X())
		binary.Write(r, binary.BigEndian, i.Y())
		r.Write(p.Payload())
		_, err = io.Copy(w, r)
	}
	return err
}
