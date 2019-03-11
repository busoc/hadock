package vmu

import (
  "encoding/binary"
  "hash"
)

type hrdlSum struct {
	sum uint32
}

func SumHRDL() hash.Hash32 {
	var v hrdlSum
	return &v
}

func SumHRD(bs []byte) uint32 {
	var h hrdlSum
	h.Write(bs)
	return h.Sum32()
}

func (h *hrdlSum) Size() int      { return 4 }
func (h *hrdlSum) BlockSize() int { return 32 }
func (h *hrdlSum) Reset()         { h.sum = 0 }

func (h *hrdlSum) Sum(bs []byte) []byte {
	h.Write(bs)
	vs := make([]byte, h.Size())
	binary.LittleEndian.PutUint32(vs, h.sum)
	return vs
}

func (h *hrdlSum) Sum32() uint32 {
	return h.sum
}

func (h *hrdlSum) Write(bs []byte) (int, error) {
	for _, b := range bs {
		h.sum += uint32(b)
	}
	return len(bs), nil
}
