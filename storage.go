package hadock

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/busoc/panda"
)

const (
	BAD = ".bad"
	XML = ".xml"
)

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

func NewLocalStorage(d, s *Archiver, g int, raw, rem bool) (Storage, error) {
	if d == nil {
		return nil, fmt.Errorf("data location not provided")
	}
	i, err := os.Stat(d.Base)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", d)
	}
	d.Levels = checkLevels(d.Levels, []string{LevelClassic, LevelVMUTime})
	d.Interval = g
	if s != nil {
		i, err := os.Stat(s.Base)
		if err != nil {
			return nil, err
		}
		if !i.IsDir() {
			return nil, fmt.Errorf("%s: not a directory", d)
		}
		s.Levels = checkLevels(s.Levels, []string{LevelClassic, LevelACQTime})
		s.Interval = g
	}

	f := &filestore{data: d, share: s, remove: rem}
	if raw {
		f.encode = encodeRawPacket
	} else {
		f.encode = func(w io.Writer, p panda.HRPacket) error {
			return p.Export(w, "")
		}
	}
	return f, nil
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

type filestore struct {
	data   *Archiver
	share  *Archiver
	remove bool
	encode func(io.Writer, panda.HRPacket) error
}

func (f *filestore) Store(i uint8, p panda.HRPacket) error {
	var w bytes.Buffer
	filename := p.Filename()
	badname := filename + BAD
	if err := f.encode(&w, p); err != nil {
		return fmt.Errorf("%s not written: %s", filename, err)
	}
	dir, err := f.data.Prepare(i, p)
	if err != nil {
		return err
	}
	if !p.IsRealtime() && f.remove {
		os.Remove(path.Join(dir, badname))
	}
	file := path.Join(dir, filename)
	if err := ioutil.WriteFile(file, w.Bytes(), 0644); err != nil {
		return err
	}
	if err := f.linkToShare(file, i, p); err != nil {
		return err
	}
	if p, ok := p.(*panda.Image); ok {
		return f.writeMetadata(dir, i, p)
	}
	return nil
}

func (f *filestore) writeMetadata(dir string, i uint8, p *panda.Image) error {
	var w bytes.Buffer

	e := xml.NewEncoder(&w)
	e.Indent("", "\t")

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
	if err := e.Encode(m); err != nil {
		return err
	}
	if w.Len() == 0 {
		return nil
	}

	filename := p.Filename() + XML
	badname := filename + BAD
	if !p.IsRealtime() && f.remove {
		os.Remove(path.Join(dir, badname))
	}
	file := path.Join(dir, filename)
	if err := ioutil.WriteFile(file, w.Bytes(), 0644); err != nil {
		return err
	}
	return f.linkToShare(file, i, p)
}

func (f *filestore) linkToShare(link string, i uint8, p panda.HRPacket) error {
	if f.share == nil {
		return nil
	}
	dir, err := f.share.Prepare(i, p)
	if err != nil {
		return err
	}
	filename := path.Base(link)
	badname := filename + BAD

	os.Remove(path.Join(dir, filename))
	if !p.IsRealtime() && f.remove {
		os.Remove(path.Join(dir, badname))
	}
	return os.Link(link, path.Join(dir, filename))
}

type Archiver struct {
	Levels   []string `toml:"levels"`
	Base     string   `toml:"location"`
	Time     string   `toml:"time"`
	Interval int      `toml:"-"`
}

func (a Archiver) Prepare(i uint8, p panda.HRPacket) (string, error) {
	var t time.Time
	switch strings.ToLower(a.Time) {
	case "vmu", "":
		t = getVMUTime(p)
	case "acq":
		t = getACQTime(p)
	default:
	}
	if t.IsZero() {
		t = p.Timestamp()
	}
	base := prepareDirectory(a.Base, a.Levels, a.Interval, i, p, t)
	if err := os.MkdirAll(base, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	return base, nil
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
	case TEST:
		base = path.Join(base, "TEST")
	case SIM1, SIM2:
		base = path.Join(base, "SIM"+fmt.Sprint(i))
	case OPS:
		base = path.Join(base, "OPS")
	default:
		base = path.Join(base, "DATA-"+fmt.Sprint(i))
	}
	return base
}

type multistore struct {
	ms []Storage
}

func (m multistore) Store(i uint8, p panda.HRPacket) error {
	var err error
	for _, s := range m.ms {
		if e := s.Store(i, p); e != nil {
			err = e
		}
	}
	return err
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
