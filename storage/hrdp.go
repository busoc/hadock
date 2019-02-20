package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/busoc/panda"
	"github.com/midbel/roll"
)

type hrdpstore struct {
	writer io.WriteCloser
	encode func(io.Writer, uint8, panda.HRPacket) error
}

// instance (1) + type (1) + mode (1) + origin (1) + sequence (4) + when (4) + upi (32) + data (len(payload))
const hrdpHeaderSize = 44

func NewHRDPStorage(o Options) (Storage, error) {
	i, err := os.Stat(o.Location)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", o.Location)
	}
	var h hrdpstore
	os := roll.Options{
		MaxSize: 64<<20,
		Interval: time.Duration(o.Interval) * time.Second,
		Timeout: time.Duration(o.Timeout) * time.Second,
		KeepEmpty: false,
		Next: func(i int, w time.Time) (string, error) {
			y := fmt.Sprintf("%04d", w.Year())
			d := fmt.Sprintf("%03d", w.YearDay())
			h := fmt.Sprintf("%02d", w.Hour())

			n := fmt.Sprintf("hdk_%06d_%02d-%02d.bin", i, w.Minute(), w.Second())
			return filepath.Join(y, d, h, n), nil
		},
	}
	switch strings.ToLower(o.Format) {
	case "hadock", "hdk":
		h.encode = encodeBinary
	case "hrdp", "rt", "vmu", "":
		h.encode = encodeHRDP
	default:
		return nil, fmt.Errorf("unsupported format %q", o.Format)
	}
	h.writer, err = roll.Buffer(o.Location, os)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (h *hrdpstore) Close() error {
	return h.writer.Close()
}

func (h *hrdpstore) Store(i uint8, p panda.HRPacket) error {
	return h.encode(h.writer, i, p)
}

func encodeBinary(ws io.Writer, i uint8, p panda.HRPacket) error {
	o, err := strconv.ParseUint(p.Origin(), 16, 8)
	if err != nil {
		return err
	}
	upi := make([]byte, 32)
	copy(upi, []byte(getUPI(p)))

	var w, b bytes.Buffer
	if err := encodeRawPacket(&b, p); err != nil {
		return err
	}
	binary.Write(&w, binary.BigEndian, uint32(b.Len() + hrdpHeaderSize))
	binary.Write(&w, binary.BigEndian, i)
	binary.Write(&w, binary.BigEndian, p.Stream())
	binary.Write(&w, binary.BigEndian, p.IsRealtime())
	binary.Write(&w, binary.BigEndian, uint8(o))
	binary.Write(&w, binary.BigEndian, p.Sequence())
	binary.Write(&w, binary.BigEndian, uint32(p.Timestamp().Unix()))
	w.Write(upi)

	io.Copy(&w, &b)

	// h.mu.Lock()
	// defer h.mu.Unlock()
	// _, err = io.CopyBuffer(h.writer, io.MultiReader(&w, &b), h.buffer)
	_, err = ws.Write(w.Bytes())
	return err
}

func encodeHRDP(w io.Writer, i uint8, p panda.HRPacket) error {
	return nil
}
