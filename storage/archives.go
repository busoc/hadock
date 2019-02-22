package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/busoc/panda"
	"github.com/midbel/roll"
)

const TAR = ".tar"

type tarstore struct {
	Control

	datadir string
	tardir  *dirmaker

	options roll.Options

	mu     sync.Mutex
	caches map[string]*roll.Tarball
}

func NewArchiveStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	dm := dirmaker{
		Levels:   checkLevels(o.Levels, []string{LevelClassic, LevelVMUTime}),
		Base:     "",
		Time:     o.Epoch,
		Interval: o.Interval,
	}
	opt := roll.Options{
		MaxSize:  1024,
		Interval: time.Duration(o.Interval) * time.Second,
		Timeout:  time.Duration(o.Timeout) * time.Second,
	}
	t := tarstore{
		Control: o.Control,
		datadir: o.Location,
		options: opt,
		tardir:  &dm,
		caches:  make(map[string]*roll.Tarball),
	}
<<<<<<< HEAD
	for _, o := range o.Shares {
		k, err := newLinkStorage(*o)
		if err != nil {
			return nil, err
		}
		s.links = append(s.links, *k)
	}
	return s, nil
=======
	return &t, nil
>>>>>>> iss839
}

func (t *tarstore) Store(i uint8, p panda.HRPacket) error {
	if !t.Can(p) {
		return nil
	}
	k := cacheKey(i, p)
	w, ok := t.caches[k]
	if !ok {
		opt := t.options
		opt.Next = nextFunc(k, p.Origin())
		tb, err := roll.Tar(t.datadir, opt)
		if err != nil {
<<<<<<< HEAD
			return nil, err
		}
		if err := t.linkToShare(path.Join(dir, p.Filename()+TAR), i, p); err != nil {
			return nil, err
		}
		tf = &tarfile{
			Closer: f,
			buffer: bufio.NewWriter(f),
			data:   t.tardir,
		}
		tf.writer = tar.NewWriter(tf.buffer)
		t.files[o] = tf
		go t.flushFile(o, time.Second*time.Duration(t.tardir.Interval))

	}
	return tf, nil
}

func (t *tarstore) linkToShare(link string, i uint8, p panda.HRPacket) error {
	for _, s := range t.links {
		if err := s.Link(link, i, p); err != nil {
=======
>>>>>>> iss839
			return err
		}
		w = tb
		t.mu.Lock()
		t.caches[k] = w
		t.mu.Unlock()
	}
<<<<<<< HEAD
	return nil
}

func (t *tarstore) flushFile(o string, d time.Duration) {
	<-time.After(d)
	t.mu.Lock()
	defer t.mu.Unlock()

	f, ok := t.files[o]
	if !ok {
		return
	}
	f.Close()
	delete(t.files, o)
}

type tarfile struct {
	mu sync.Mutex
	io.Closer
	buffer *bufio.Writer
	writer *tar.Writer

	data dirmaker
}

func (t *tarfile) Close() error {
	t.writer.Close()
	t.buffer.Flush()
	return t.Closer.Close()
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
=======
	var buf bytes.Buffer
	if err := encodeRawPacket(&buf, p); err != nil {
		return err
>>>>>>> iss839
	}
	dir, _ := t.tardir.Prepare(i, p)
	h := roll.Header{
		Name:    filepath.Join(dir, p.Filename()),
		Size:    int64(buf.Len()),
		ModTime: p.Timestamp(),
		Gid:     1000,
		Uid:     1000,
		Mode:    0644,
	}
	if err := w.Write(&h, buf.Bytes()); err != nil {
		return err
	}
	if p, ok := p.(*panda.Image); ok {
		return t.storeMetadata(w, i, p)
	}
	return nil
}

func (t *tarstore) storeMetadata(tb *roll.Tarball, i uint8, p *panda.Image) error {
	var w bytes.Buffer
	if err := encodeMetadata(&w, p); err != nil {
		return err
	}
	dir, _ := t.tardir.Prepare(i, p)
	h := roll.Header{
		Name:    filepath.Join(dir, p.Filename() + XML),
		Size:    int64(w.Len()),
		ModTime: p.Timestamp(),
		Gid:     1000,
		Uid:     1000,
		Mode:    0644,
	}
	return tb.Write(&h, w.Bytes())
}

func nextFunc(k string, origin string) roll.NextFunc {
	var iter int
	return func(i int, n time.Time) (string, error) {
		iter++
		f := fmt.Sprintf("%s_%s-%06d.tar", origin, n.Format("20180102150405"), iter)
		return filepath.Join(k, f), nil
	}
}

func cacheKey(i uint8, p panda.HRPacket) string {
	base := instanceDir("", i)
	base = modeDir(base, p)
	base = typeDir(base, p)
	return filepath.Join(base, p.Origin())
}
