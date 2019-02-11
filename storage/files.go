package storage

import (
	"bytes"
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

type filestore struct {
	Control

	data   *dirmaker
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
	dm := dirmaker{
		Levels:   checkLevels(o.Levels, []string{LevelClassic, LevelVMUTime}),
		Base:     o.Location,
		Time:     o.Epoch,
		Interval: o.Interval,
		cache:    make(map[string]time.Time),
	}
	go dm.clean()

	s := filestore{
		Control: o.Control,
		rembad:  !o.KeepBad,
		data:    &dm,
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
	if !p.IsRealtime() && f.rembad {
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
	data   *dirmaker
}

func newLinkStorage(o Options) (*linkstore, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	dm := dirmaker{
		Levels:   checkLevels(o.Levels, []string{LevelClassic, LevelACQTime}),
		Base:     o.Location,
		Time:     o.Epoch,
		Interval: o.Interval,
		cache:    make(map[string]time.Time),
	}
	go dm.clean()

	switch o.Link {
	case "", "hard", "soft":
	default:
		return nil, fmt.Errorf("invalid link type %s", o.Link)
	}
	k := linkstore{
		link:   o.Link,
		rembad: !o.KeepBad,
		data:   &dm,
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
