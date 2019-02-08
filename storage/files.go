package storage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/busoc/panda"
)

const (
	BAD = ".bad"
	XML = ".xml"
)

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
	d.cache = make(map[string]time.Time)
	go d.clean()
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
		s.cache = make(map[string]time.Time)

		go s.clean()
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
