package storage

import (
	"bufio"
	"bytes"
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
	datadir string
	tick    <-chan time.Time

	mu     sync.Mutex
	writer io.WriteCloser

	last time.Time
}

func NewHRDPStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	h := hrdpstore{
		datadir: o.Location,
		tick:    time.Tick(time.Minute * 5),
	}
	p, err := timepath(o.Location)
	if err != nil {
		return nil, err
	}
	h.writer, err = createFile(p)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (h *hrdpstore) Store(i uint8, p panda.HRPacket) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	select {
	case <-h.tick:
		h.writer.Close()
		p, err := timepath(h.datadir)
		if err != nil {
			return err
		}
		h.writer, err = createFile(p)
		if err != nil {
			return err
		}
	default:
		h.last = time.Now()
	}
	o, err := strconv.ParseUint(p.Origin(), 16, 8)
	if err != nil {
		return err
	}
	upi := make([]byte, 32)
	copy(upi, []byte(getUPI(p)))
	// instance (1) + type (1) + mode (1) + origin (1) + upi (???) + data (???)
	var w bytes.Buffer
	binary.Write(&w, binary.BigEndian, i)
	binary.Write(&w, binary.BigEndian, p.Stream())
	binary.Write(&w, binary.BigEndian, p.IsRealtime())
	binary.Write(&w, binary.BigEndian, uint8(o))
	w.Write(upi)
	w.Write(p.Payload())

	binary.Write(h.writer, binary.BigEndian, uint32(w.Len()))
	_, err = io.Copy(h.writer, &w)
	return err
}

type file struct {
	*os.File
	buffer *bufio.Writer
}

func timepath(p string) (string, error) {
	n := time.Now()
	y := fmt.Sprintf("%04d")
	d := fmt.Sprintf("%03d")
	h := fmt.Sprintf("%02d")
	m := n.Truncate(time.Minute * 5)

	dir := path.Join(p, y, d, h)
	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	return path.Join(dir, fmt.Sprintf("hdk_%02d.dat", m)), nil
}

func createFile(p string) (*file, error) {
	f, err := os.Create(p)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriter(f)
	return &file{File: f, buffer: w}, nil
}

func (f *file) Write(bs []byte) (int, error) {
	return f.buffer.Write(bs)
}

func (f *file) Close() error {
	f.buffer.Flush()
	return f.File.Close()
}
