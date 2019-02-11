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

	o := p.Origin()
	tf, ok := t.files[o]
	if !ok {
		dir, err := t.datadir.Prepare(i, p)
		if err != nil {
			return nil, err
		}
		f, err := os.Create(path.Join(dir, p.Filename()+TAR))
		if err != nil {
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

func (t *tarstore) flushFile(o string, d time.Duration) {
	<-time.After(d)
	t.mu.Lock()
	defer t.mu.Unlock()

	f, ok := t.files[o]
	if !ok {
		return
	}
	if err := f.Close(); err != nil {
		fmt.Println(err)
	}
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
	}
	dir, _ := t.data.Prepare(i, p)
	h := tar.Header{
		Name:    path.Join(dir, p.Filename()),
		Size:    int64(w.Len()),
		ModTime: when,
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.writer.WriteHeader(&h); err != nil {
		return err
	}
	_, err := io.CopyN(t.writer, &w, h.Size)
	return err
}
