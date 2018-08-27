package hadock

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/busoc/panda"
)

const (
	BAD = ".bad"
	XML = ".xml"
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

func NewLocalStorage(d, h string, g int, raw, rem bool) (Storage, error) {
	i, err := os.Stat(d)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", d)
	}
	f := &filestore{datadir: d, harddir: h, granul: g, remove: rem}
	if raw {
		f.encode = encodeRawPacket
	} else {
		f.encode = func(w io.Writer, p panda.HRPacket) error {
			return p.Export(w, "")
		}
	}
	return f, nil
}

func NewHTTPStorage(d string, g int) (Storage, error) {
	u, err := url.Parse(d)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" || u.Scheme != "https" {
		return nil, fmt.Errorf("%s: not a valid url", d)
	}
	return &httpstore{*u, g}, nil
}

type filestore struct {
	datadir, harddir string
	granul           int
	remove           bool
	encode           func(io.Writer, panda.HRPacket) error
}

func (f *filestore) mkdirall(dir string, i uint8, p panda.HRPacket, isHardLink bool) (string, error) {
	dir, err := joinPath(dir, p, i, f.granul, isHardLink)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	return dir, nil
}

func (f *filestore) Store(i uint8, p panda.HRPacket) error {
	w := new(bytes.Buffer)
	filename := p.Filename()
	badname := filename + BAD
	if err := f.encode(w, p); err != nil {
		return fmt.Errorf("%s not written: %s", filename, err)
	}
	dir, err := f.mkdirall(f.datadir, i, p, false)
	if err != nil {
		return err
	}
	if f.remove {
		os.Remove(path.Join(dir, badname))
	}
	if err := ioutil.WriteFile(path.Join(dir, filename), w.Bytes(), 0644); err != nil {
		return err
	}
	if n, err := os.Stat(f.harddir); err == nil && n.IsDir() {
		hard, err := f.mkdirall(f.harddir, i, p, true)
		if err != nil {
			return err
		}
		os.Remove(path.Join(hard, filename))
		if f.remove {
			os.Remove(path.Join(hard, badname))
		}
		if err := os.Link(path.Join(dir, filename), path.Join(hard, filename)); err != nil {
			return err
		}
	}
	if p, ok := p.(*panda.Image); ok {
		w := new(bytes.Buffer)
		e := xml.NewEncoder(w)
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
		filename += XML
		badname += XML
		if f.remove {
			os.Remove(path.Join(dir, badname))
		}
		if err := ioutil.WriteFile(path.Join(dir, filename), w.Bytes(), 0644); err != nil {
			return err
		}
		if s, err := os.Stat(f.harddir); err == nil && s.IsDir() {
			hard, err := f.mkdirall(f.harddir, i, p, true)
			if err != nil {
				return err
			}
			os.Remove(path.Join(hard, filename))
			if f.remove {
				os.Remove(path.Join(hard, badname))
			}
			if err := os.Link(path.Join(dir, filename), path.Join(hard, filename)); err != nil {
				return err
			}
		}
	}
	return nil
}

type httpstore struct {
	location url.URL
	granul   int
}

func (h *httpstore) Store(i uint8, p panda.HRPacket) error {
	var err error
	u := h.location
	u.Path, err = joinPath(u.Path, p, i, h.granul, false)
	if err != nil {
		return err
	}
	w := new(bytes.Buffer)
	if err := p.Export(w, ""); err != nil {
		return err
	}
	rs, err := http.Post(u.String(), "application/octet-stream", w)
	if err != nil {
		return err
	}
	if rs.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf(http.StatusText(rs.StatusCode))
	}
	return nil
}

type hrdpstore struct {
	datadir  string
	payload  uint8
	syncword uint32
	channels []panda.Channel
	instance uint8  //OPS, TEST, SIM1, SIM2
	mode     string //realtime, playback

	buf *bytes.Buffer
}

func NewHRDPStorage(d string, id uint8) (Storage, error) {
	i, err := os.Stat(d)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", d)
	}
	return &hrdpstore{datadir: d, payload: id, syncword: Preamble, buf: new(bytes.Buffer)}, nil
}

func (h *hrdpstore) Store(i uint8, p panda.HRPacket) error {
	w := new(bytes.Buffer)
	o, _ := strconv.ParseUint(p.Origin(), 0, 8)

	var v *panda.VMUHeader
	switch p := p.(type) {
	case *panda.Table:
		v = p.VMUHeader
	case *panda.Image:
		v = p.VMUHeader
	}
	now, gen := time.Now(), v.Timestamp()
	if t := gen.Truncate(time.Minute * 5); t.Minute()%5 == 0 {
		n := fmt.Sprintf("rt_%02d_%02d.dat", t.Minute()-5, t.Minute()-1)
		bs := h.buf.Bytes()
		h.buf.Reset()
		p, _ := joinPathHRDP(h.datadir, t, i)
		if err := os.MkdirAll(p, 0755); err != nil {
			return err
		}
		if err := ioutil.WriteFile(path.Join(p, n), bs, 0644); err != nil {
			return err
		}
	}

	bs, err := p.Bytes()
	if err != nil {
		return err
	}
	length := panda.HRDPHeaderLength + panda.HRDLSyncLength
	binary.Write(w, binary.LittleEndian, uint32(length+len(bs)))
	binary.Write(w, binary.LittleEndian, uint16(0))
	binary.Write(w, binary.LittleEndian, h.payload)
	binary.Write(w, binary.LittleEndian, uint8(o))
	binary.Write(w, binary.LittleEndian, gen.Unix())
	binary.Write(w, binary.LittleEndian, uint8(0))
	binary.Write(w, binary.LittleEndian, now.Unix())
	binary.Write(w, binary.LittleEndian, uint8(0))
	binary.Write(w, binary.BigEndian, h.syncword)
	binary.Write(w, binary.BigEndian, uint32(len(bs)))
	w.Write(bs)

	if _, err := io.Copy(h.buf, w); err != nil {
		return err
	}
	return nil
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

func joinPath(base string, v panda.HRPacket, i uint8, g int, a bool) (string, error) {
	switch i {
	case TEST:
		base = path.Join(base, "TEST")
	case SIM1, SIM2:
		base = path.Join(base, "SIM"+fmt.Sprint(i))
	case OPS:
		base = path.Join(base, "OPS")
	default:
		base = path.Join(base, "DATA")
	}
	var t time.Time
	switch v := v.(type) {
	case *panda.Table:
		base, t = path.Join(base, "sciences"), v.VMUHeader.Timestamp()
	case *panda.Image:
		base, t = path.Join(base, "images"), v.VMUHeader.Timestamp()
	}
	if v.IsRealtime() {
		base = path.Join(base, "realtime", v.Origin())
	} else {
		base = path.Join(base, "playback", v.Origin())
	}
	if t.IsZero() || a {
		t = v.Timestamp()
	}

	return joinPathTime(base, t, g, a), nil
}

func joinPathHRDP(base string, t time.Time, i uint8) (string, error) {
	switch i {
	case TEST:
		base = path.Join(base, "TEST")
	case SIM1, SIM2:
		base = path.Join(base, "SIM"+fmt.Sprint(i))
	case OPS:
		base = path.Join(base, "OPS")
	default:
		base = path.Join(base, "DATA")
	}
	return joinPathTime(base, t, 0, false), nil
}

func joinPathTime(base string, t time.Time, g int, a bool) string {
	if !a {
		t = panda.AdjustGenerationTime(t.Unix())
	}
	y := fmt.Sprintf("%04d", t.Year())
	d := fmt.Sprintf("%03d", t.YearDay())
	h := fmt.Sprintf("%02d", t.Hour())
	base = path.Join(base, y, d, h)
	if m := t.Truncate(time.Second * time.Duration(g)); g > 0 {
		base = path.Join(base, fmt.Sprintf("%02d", m.Minute()))
	}
	return base
}
