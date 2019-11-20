package storage

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/busoc/panda"
)

const (
	BAD = ".bad"
	XML = ".xml"
)

type filestore struct {
	Control

	data   Directory
	rembad bool
	encode func(io.Writer, panda.HRPacket) error

	links []linkstore
}

func NewLocalStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	dm := NewDirectory(o.Location, o.Epoch, o.Levels, o.Interval)

	s := filestore{
		Control: o.Control,
		rembad:  !o.KeepBad,
		data:    dm,
	}
	for _, o := range o.Shares {
		k, err := newLinkStorage(*o)
		if err != nil {
			return nil, err
		}
		s.links = append(s.links, *k)
	}
	switch o.Format {
	case "raw":
		s.encode = encodeRawPacket
	default:
		s.encode = func(w io.Writer, p panda.HRPacket) error {
			return p.Export(w, "")
		}
	}
	return &s, nil
}

func (f *filestore) Store(i uint8, p panda.HRPacket) error {
	if !f.Can(p) {
		return nil
	}
	dir, err := f.data.Prepare(i, p)
	if err != nil {
		return err
	}
	var w bytes.Buffer
	filename := p.Filename()
	if f.rembad && !panda.Valid(p) {
		i, err := os.Stat(path.Join(dir, filename))
		if err == nil && i.Mode().IsRegular() {
			return nil
		}
	}
	badname := filename + BAD
	if err := f.encode(&w, p); err != nil {
		return fmt.Errorf("%s not written: %s", filename, err)
	}

	if f.rembad {
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
	if err := encodeMetadata(&w, p); err != nil {
		return err
	}
	if w.Len() == 0 {
		return nil
	}

	filename := p.Filename() + XML
	badname := filename + BAD
	if !p.IsRealtime() && f.rembad {
		os.Remove(path.Join(dir, badname))
	}
	file := path.Join(dir, filename)
	if err := ioutil.WriteFile(file, w.Bytes(), 0644); err != nil {
		return err
	}
	return f.linkToShare(file, i, p)
}

func (f *filestore) linkToShare(link string, i uint8, p panda.HRPacket) error {
	for _, s := range f.links {
		if err := s.Link(link, i, p); err != nil {
			return err
		}
	}
	return nil
}

type linkstore struct {
	link   string
	rembad bool
	data   Directory
}

func newLinkStorage(o Options) (*linkstore, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}

	levels := checkLevels(o.Levels, []string{LevelClassic, LevelACQTime})
	dm := NewDirectory(o.Location, o.Epoch, levels, o.Interval)

	switch o.Link {
	case "", "hard", "soft":
	default:
		return nil, fmt.Errorf("invalid link type %s", o.Link)
	}
	k := linkstore{
		link:   o.Link,
		rembad: !o.KeepBad,
		data:   dm,
	}
	return &k, nil
}

func (s linkstore) Link(link string, i uint8, p panda.HRPacket) error {
	dir, err := s.data.Prepare(i, p)
	if err != nil {
		return err
	}
	filename := path.Base(link)
	badname := filename + BAD

	os.Remove(path.Join(dir, filename))
	if !p.IsRealtime() && s.rembad {
		os.Remove(path.Join(dir, badname))
	}
	switch s.link {
	case "hard", "":
		err = os.Link(link, path.Join(dir, filename))
	case "soft":
		err = os.Symlink(link, path.Join(dir, filename))
	}
	return err
}
