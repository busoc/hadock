package storage

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/busoc/panda"
)

const TAR = ".tar"

type tarstore struct {
	Control

	datadir dirmaker
	tardir  dirmaker

	mu    sync.Mutex
	files map[string]*tarfile

	timeout time.Duration
	links   []linkstore
}

func NewArchiveStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	datadir := dirmaker{
		Levels:   checkLevels(o.Levels, []string{LevelClassic, LevelVMUTime}),
		Time:     o.Epoch,
		Interval: 0,
		Base:     o.Location,
		cache:    make(map[string]time.Time),
	}
	tardir := dirmaker{
		Levels:   checkLevels(o.Levels, []string{LevelClassic, LevelVMUTime}),
		Time:     o.Epoch,
		Interval: o.Interval,
	}
	if tardir.Interval <= 0 {
		tardir.Interval = 60
	}
	s := &tarstore{
		Control: o.Control,
		datadir: datadir,
		tardir:  tardir,
		files:   make(map[string]*tarfile),
	}
	if o.Timeout > 0 {
		s.timeout = time.Second * time.Duration(o.Timeout)
	} else {
		s.timeout = time.Minute
	}
	for _, o := range o.Shares {
		k, err := newLinkStorage(*o)
		if err != nil {
			return nil, err
		}
		s.links = append(s.links, *k)
	}
	return s, nil
}

func (t *tarstore) Store(i uint8, p panda.HRPacket) error {
	if !t.Can(p) {
		return nil
	}
	tw, err := t.writer(i, p)
	if err != nil {
		return err
	}
	return tw.storePacket(i, p)
}

func (t *tarstore) writer(i uint8, p panda.HRPacket) (*tarfile, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// o := p.Origin()

	o := makeKey(i, p)
	tf, ok := t.files[o]
	if ok && tf.shouldClose(p) {
		tf.Close()
		delete(t.files, o)
		ok = !ok
	}
	if !ok {
		dir, err := t.datadir.Prepare(i, p)
		if err != nil {
			return nil, err
		}
		f, err := os.Create(path.Join(dir, p.Filename()+TAR))
		if err != nil {
			return nil, err
		}
		if err := t.linkToShare(path.Join(dir, p.Filename()+TAR), i, p); err != nil {
			return nil, err
		}
		var when time.Time
		switch t.tardir.Time {
		case "vmu", "":
			when = getVMUTime(p)
		case "acq":
			when = getACQTime(p)
		}
		tf = &tarfile{
			first:  when,
			ttl:    time.Duration(t.tardir.Interval) * time.Second,
			file:   f,
			buffer: bufio.NewWriter(f),
			data:   t.tardir,
		}
		tf.writer = tar.NewWriter(tf.buffer)
		t.files[o] = tf
		go t.flushFile(o, t.timeout)

	}
	return tf, nil
}

func makeKey(i uint8, p panda.HRPacket) string {
	base := p.Origin()
	base = instanceDir(base, i)
	base = typeDir(base, p)
	base = modeDir(base, p)

	return base
}

func (t *tarstore) linkToShare(link string, i uint8, p panda.HRPacket) error {
	for _, s := range t.links {
		if err := s.Link(link, i, p); err != nil {
			return err
		}
	}
	return nil
}

func (t *tarstore) flushFile(o string, d time.Duration) {
	tick := time.NewTicker(d)
	defer tick.Stop()
	for n := range tick.C {
		t.mu.Lock()

		var done bool
		if f, ok := t.files[o]; ok {
			if n.Sub(f.last) > d {
				f.Close()
				if f.count == 0 {
					os.Remove(f.file.Name())
				}
				delete(t.files, o)
				done = true
			}
		}
		t.mu.Unlock()
		if done {
			return
		}
	}
}

type tarfile struct {
	mu     sync.Mutex
	file   *os.File
	buffer *bufio.Writer
	writer *tar.Writer

	ttl   time.Duration
	first time.Time

	data  dirmaker
	last  time.Time
	count int
}

func (t *tarfile) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.writer.Close()
	t.buffer.Flush()
	return t.file.Close()
}

func (t *tarfile) shouldClose(p panda.HRPacket) bool {
	var when time.Time
	switch t.data.Time {
	case "vmu", "":
		when = getVMUTime(p)
	case "acq":
		when = getACQTime(p)
	}
	return when.Sub(t.first) >= t.ttl
}

func (t *tarfile) storePacket(i uint8, p panda.HRPacket) error {
	var w bytes.Buffer
	if err := encodeRawPacket(&w, p); err != nil {
		return fmt.Errorf("%s not written: %s", p.Filename(), err)
	}
	var when time.Time
	switch t.data.Time {
	case "vmu", "":
		when = getVMUTime(p)
	case "acq":
		when = getACQTime(p)
	}
	dir, _ := t.data.Prepare(i, p)
	h := tar.Header{
		Typeflag: tar.TypeReg,
		Name:     path.Join(dir, p.Filename()),
		Size:     int64(w.Len()),
		ModTime:  when,
		Uid:      1000,
		Gid:      1000,
		Format:   tar.FormatGNU,
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.last = time.Now()
	t.count++
	if err := t.writer.WriteHeader(&h); err != nil {
		return err
	}
	_, err := io.CopyN(t.writer, &w, h.Size)
	if err != nil {
		return err
	}
	if p, ok := p.(*panda.Image); ok {
		return t.storeMetadata(i, p)
	}
	return err
}

func (t *tarfile) storeMetadata(i uint8, p *panda.Image) error {
	var w bytes.Buffer
	if err := encodeMetadata(&w, p); err != nil {
		return err
	}
	if w.Len() == 0 {
		return nil
	}
	var when time.Time
	switch t.data.Time {
	case "vmu", "":
		when = getVMUTime(p)
	case "acq":
		when = getACQTime(p)
	}
	dir, _ := t.data.Prepare(i, p)
	h := tar.Header{
		Typeflag: tar.TypeReg,
		Name:     path.Join(dir, p.Filename()+XML),
		Size:     int64(w.Len()),
		ModTime:  when,
		Uid:      1000,
		Gid:      1000,
		Format:   tar.FormatGNU,
	}
	if err := t.writer.WriteHeader(&h); err != nil {
		return err
	}
	_, err := io.CopyN(t.writer, &w, h.Size)
	return err
}
