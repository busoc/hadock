package storage

import (
	"bytes"
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/busoc/panda"
)

type hrdpstore struct {
	writer io.WriteCloser
}

func NewHRDPStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	var h hrdpstore
	h.writer, err = createFile(o.Location)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (h *hrdpstore) Close() error {
	return h.writer.Close()
}

func (h *hrdpstore) Store(i uint8, p panda.HRPacket) error {
	o, err := strconv.ParseUint(p.Origin(), 16, 8)
	if err != nil {
		return err
	}
	upi := make([]byte, 32)
	copy(upi, []byte(getUPI(p)))
	// instance (1) + type (1) + mode (1) + origin (1) + sequence (4) + upi (32) + data (len(payload))
	var w bytes.Buffer
	binary.Write(&w, binary.BigEndian, i)
	binary.Write(&w, binary.BigEndian, p.Stream())
	binary.Write(&w, binary.BigEndian, p.IsRealtime())
	binary.Write(&w, binary.BigEndian, uint8(o))
	binary.Write(&w, binary.BigEndian, p.Sequence())
	binary.Write(&w, binary.BigEndian, uint32(p.Timestamp().Unix()))
	w.Write(upi)
	if err := encodeRawPacket(&w, p); err != nil {
		return err
	}
	binary.Write(h.writer, binary.BigEndian, uint32(w.Len()))
	_, err = io.Copy(h.writer, &w)
	return err
}

type file struct {
	ticker  *time.Ticker
	datadir string

	mu sync.Mutex
	inner *os.File
	writer *bufio.Writer
	written chan int
}

func createFile(d string) (io.WriteCloser, error) {
	t := time.NewTicker(time.Minute*5)
	f := file{
		datadir: d,
		ticker: t,
		written: make(chan int),
	}
	if err := f.createFile(time.Now()); err != nil {
		return nil, err
	}
	go f.rotate()
	return &f, nil
}

func (f *file) Write(bs []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.writer.Write(bs)
	if err == nil {
		f.written <- n
	}
	return n, err
}

func (f *file) Close() error {
	f.ticker.Stop()
	return nil
	// return f.flushAndClose()
}

func (f *file) rotate() {
	var written int
	ten := time.Second*10
	timeout := time.NewTimer(ten)
	for {
		select {
		case n := <- f.ticker.C:
			f.mu.Lock()
			f.flushAndClose(written)
			f.createFile(n)
			f.mu.Unlock()
			if !timeout.Stop() {
				<-timeout.C
			}
			timeout.Reset(ten)
		case n := <- f.written:
			if !timeout.Stop() {
				<-timeout.C
			}
			timeout.Reset(ten)
			written+=n
		case <-timeout.C:
			f.mu.Lock()
			f.flushAndClose(written)
			f.mu.Unlock()
		}
	}
	f.flushAndClose(written)
}

func (f *file) createFile(n time.Time) error {
	p, err := timepath(f.datadir, n)
	if err != nil {
		return err
	}
	f.inner, err = os.Create(p)
	if err != nil {
		return err
	}
	if f.writer == nil {
		f.writer = bufio.NewWriter(f.inner)
	} else {
		f.writer.Reset(f.inner)
	}
	return nil
}

func (f *file) flushAndClose(written int) error {
	err := f.writer.Flush()
	if err := f.inner.Close(); err != nil {
		return err
	}
	if written == 0 {
		os.Remove(f.inner.Name())
	}
	return err
}

func timepath(p string, n time.Time) (string, error) {
	if n.IsZero() {
		n = time.Now()
	}
	y := fmt.Sprintf("%04d", n.Year())
	d := fmt.Sprintf("%03d", n.YearDay())
	h := fmt.Sprintf("%02d", n.Hour())
	m := n.Truncate(time.Minute * 5)

	dir := path.Join(p, y, d, h)
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	return path.Join(dir, fmt.Sprintf("hdk_%02d.dat", m.Minute())), nil
}
