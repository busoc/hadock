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
	return &t, nil
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
			return err
		}
		w = tb
		t.mu.Lock()
		t.caches[k] = w
		t.mu.Unlock()
	}
	var buf bytes.Buffer
	if err := encodeRawPacket(&buf, p); err != nil {
		return err
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
	return w.Write(&h, buf.Bytes())
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
	base := p.Origin()
	base = instanceDir(base, i)
	base = typeDir(base, p)

	return modeDir(base, p)
}
