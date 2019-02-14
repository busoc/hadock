package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
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
	return &h, nil
}

func (h *hrdpstore) Store(i uint8, p panda.HRPacket) error {
	o, err := strconv.ParseUint(p.Origin(), 16, 8)
	if err != nil {
		return err
	}
	upi := make([]byte, 32)
	copy(upi, []byte(getUPI(p)))
	// instance (1) + type (1) + mode (1) + origin (1) + upi (32) + data (len(payload))
	var w bytes.Buffer
	binary.Write(&w, binary.BigEndian, i)
	binary.Write(&w, binary.BigEndian, p.Stream())
	binary.Write(&w, binary.BigEndian, p.IsRealtime())
	binary.Write(&w, binary.BigEndian, uint8(o))
	w.Write(upi)
	if err := encodeRawPacket(&w, p); err != nil {
		return err
	}
	binary.Write(h.writer, binary.BigEndian, uint32(w.Len()))
	_, err = io.Copy(h.writer, &w)
	return err
}

func timepath(p string, n time.Time) (string, error) {
	if n.IsZero() {
		n = time.Now()
	}
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
