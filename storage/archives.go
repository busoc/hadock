package storage

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
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

	options []roll.Option
	datadir string
	tardir  *dirmaker

	mu     sync.Mutex
	caches map[string]*roll.Roller
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
	options := []roll.Option{
		roll.WithThreshold(o.MaxSize, o.MaxCount),
		roll.WithTimeout(time.Duration(o.Timeout) * time.Second),
		roll.WithInterval(time.Duration(o.Interval) * time.Second),
	}
	t := tarstore{
		Control: o.Control,
		datadir: o.Location,
		options: options,
		tardir:  &dm,
		caches:  make(map[string]*roll.Roller),
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
		tb, err := roll.Roll(nextFunc(filepath.Join(t.datadir, k, p.Origin())), t.options...)
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
	before := func(w io.Writer) error {
		dir, _ := t.tardir.Prepare(i, p)
		h := tar.Header{
			Name:    filepath.Join(dir, p.Filename()),
			Size:    int64(buf.Len()),
			ModTime: p.Timestamp(),
			Gid:     1000,
			Uid:     1000,
			Mode:    0644,
		}
		return w.(*tar.Writer).WriteHeader(&h)
	}
	if _, err := w.WriteData(buf.Bytes(), before, nil); err != nil {
		return err
	}
	// if err := w.Write(&h, buf.Bytes()); err != nil {
	// 	return err
	// }
	if p, ok := p.(*panda.Image); ok {
		return t.storeMetadata(w, i, p)
	}
	return nil
}

func (t *tarstore) storeMetadata(w *roll.Roller, i uint8, p *panda.Image) error {
	var buf bytes.Buffer
	if err := encodeMetadata(&buf, p); err != nil {
		return err
	}

	before := func(w io.Writer) error {
		dir, _ := t.tardir.Prepare(i, p)
		h := tar.Header{
			Name:    filepath.Join(dir, p.Filename()+XML),
			Size:    int64(buf.Len()),
			ModTime: p.Timestamp(),
			Gid:     1000,
			Uid:     1000,
			Mode:    0644,
		}
		return w.(*tar.Writer).WriteHeader(&h)
	}
	_, err := w.WriteData(buf.Bytes(), before, nil)
	return err
}

func nextFunc(base string) roll.NextFunc {
	return func(i int, w time.Time) (io.WriteCloser, []io.Closer, error) {
		year := fmt.Sprintf("%04d", w.Year())
		doy := fmt.Sprintf("%03d", w.YearDay())
		hour := fmt.Sprintf("%02d", w.Hour())

		datadir := filepath.Join(base, year, doy, hour)
		if err := os.MkdirAll(datadir, 0755); err != nil {
			return nil, nil, err
		}
		file := fmt.Sprintf("hdk_%s.tar", w.Format("150405"))
		wc, err := os.Create(filepath.Join(datadir, file))
		if err != nil {
			return nil, nil, err
		}
		tw := tar.NewWriter(wc)
		return tw, []io.Closer{wc}, nil
	}
}

func cacheKey(i uint8, p panda.HRPacket) string {
	base := instanceDir("", i)
	base = modeDir(base, p)
	base = typeDir(base, p)
	return filepath.Join(base, p.Origin())
}
