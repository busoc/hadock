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

	"github.com/busoc/hadock/vmu"
	"github.com/busoc/panda"
	"github.com/midbel/roll"
)

type hrdpstore struct {
	datadir string
	encode func(io.Writer, uint8, panda.HRPacket) error

	writer io.WriteCloser
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
	h := hrdpstore{datadir: o.Location}
	options := []roll.Option{
		WithTimeout(time.Duration(o.Timeout) * time.Second),
		WithInterval(time.Duration(o.Interval) * time.Second),
	}

	switch strings.ToLower(o.Format) {
	case "hrdp", "vmu":
		h.encode = encodeHRDP
	case "hadock", "hdk":
		h.encode = encodeHadock
	default:
		return nil, fmt.Errorf("unknown format %q", o.Format)
	}
	h.writer, err = roll.Roll(h.Open, options...)
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

func (h *hrdpstore) Open(_ int, w time.Time) (io.WriteCloser, []io.Closer, error) {
	year := fmt.Sprintf("%04d", w.Year())
	doy := fmt.Sprintf("%03d", w.YearDay())
	hour := fmt.Sprintf("%02d", w.Hour())

	datadir := filepath.Join(h.datadir, year, doy, hour)
	if err := os.MkdirAll(datadir, 0755); err != nil {
		return nil, nil, err
	}
	file := filepath.Join(datadir, fmt.Sprintf("hdk_%s.dat", w.Format("150405")))
  wc, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  return wc, nil, err
}

func encodeHadock(ws io.Writer, i uint8, p panda.HRPacket) error {
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
	binary.Write(&w, binary.BigEndian, uint32(b.Len()+hrdpHeaderSize))
	binary.Write(&w, binary.BigEndian, i)
	binary.Write(&w, binary.BigEndian, p.Stream())
	binary.Write(&w, binary.BigEndian, p.IsRealtime())
	binary.Write(&w, binary.BigEndian, uint8(o))
	binary.Write(&w, binary.BigEndian, p.Sequence())
	binary.Write(&w, binary.BigEndian, uint32(p.Timestamp().Unix()))
	w.Write(upi)

	io.Copy(&w, &b)

	_, err = ws.Write(w.Bytes())
	return nil
}

func encodeHRDP(w io.Writer, _ uint8, p panda.HRPacket) error {
	return vmu.EncodePacket(w, p, true)
}
