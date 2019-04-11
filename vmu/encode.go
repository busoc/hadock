package vmu

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/busoc/panda"
	"github.com/busoc/timutil"
)

func EncodePacket(ws io.Writer, p panda.HRPacket, hrdp bool) error {
	var (
		err    error
		buffer bytes.Buffer
	)
	if hrdp {
		if err := encodeHRDPHeader(&buffer, p); err != nil {
			return err
		}
	}
	switch p := p.(type) {
	case *panda.Table:
		err = encodeTable(&buffer, p)
	case *panda.Image:
		err = encodeImage(&buffer, p)
	default:
		err = fmt.Errorf("unsupported packet type %T", p)
	}
	if err != nil {
		return err
	}
	_, err = io.Copy(ws, &buffer)
	return err
}

func encodeHRDPHeader(ws io.Writer, p panda.HRPacket) error {
	size := uint32(HRDPHeaderLen + HRDLSyncLen + VMUHLen + 4)
	switch c := p.Stream(); c {
	case panda.Science:
		size += SDHLenV2
	case panda.Video1, panda.Video2:
		size += IDHLenV2
	default:
		return fmt.Errorf("unknown channel %d", c)
	}
	size += uint32(len(p.Payload()))

	binary.Write(ws, binary.LittleEndian, size)
	binary.Write(ws, binary.BigEndian, uint16(0))
	binary.Write(ws, binary.BigEndian, uint8(FSLMagic))
	binary.Write(ws, binary.BigEndian, p.Stream())

	acq := time.Now()
	now := timutil.GPSTime(acq, true)
	if g, ok := p.(interface{ Generated() time.Time }); ok {
		acq = g.Generated()
	}
	var (
		fine   uint32
		coarse uint8
	)
	fine, coarse = timutil.Split5(acq)
	binary.Write(ws, binary.BigEndian, fine)
	binary.Write(ws, binary.BigEndian, coarse)

	fine, coarse = timutil.Split5(now)
	binary.Write(ws, binary.BigEndian, fine)
	binary.Write(ws, binary.BigEndian, coarse)
	return nil
}

func encodeVMUHeader(ws io.Writer, v *panda.VMUHeader) error {
	binary.Write(ws, binary.LittleEndian, v.Channel)
	binary.Write(ws, binary.LittleEndian, v.Source)
	binary.Write(ws, binary.LittleEndian, uint16(0))
	binary.Write(ws, binary.LittleEndian, v.Sequence)
	binary.Write(ws, binary.LittleEndian, v.Coarse)
	binary.Write(ws, binary.LittleEndian, v.Fine)
	binary.Write(ws, binary.LittleEndian, uint16(0))
	return nil
}

func encodeTable(ws io.Writer, p *panda.Table) error {
	body := p.Payload()
	size := uint32(VMUHLen + SDHLenV2 + len(body) - 4)

	binary.Write(ws, binary.BigEndian, uint32(HRDLMagic))
	binary.Write(ws, binary.LittleEndian, size)

	digest := SumHRDL()
	ws = io.MultiWriter(ws, digest)
	if err := encodeVMUHeader(ws, p.VMUHeader); err != nil {
		return err
	}
	s, ok := p.SDH.(*panda.SDHv2)
	if !ok {
		return fmt.Errorf("invalid SDH version")
	}
	binary.Write(ws, binary.LittleEndian, s.Properties)
	binary.Write(ws, binary.LittleEndian, s.Sequence)
	binary.Write(ws, binary.LittleEndian, s.Originator)
	binary.Write(ws, binary.LittleEndian, s.Acquisition)
	binary.Write(ws, binary.LittleEndian, s.Auxiliary)
	binary.Write(ws, binary.LittleEndian, s.Id)
	ws.Write(s.Info[:])
	ws.Write(body)

	sum := digest.Sum32()
	binary.Write(ws, binary.LittleEndian, sum)

	return nil
}

func encodeImage(ws io.Writer, p *panda.Image) error {
	body := p.Payload()
	size := uint32(VMUHLen + SDHLenV2 + len(body) - 4)

	binary.Write(ws, binary.BigEndian, uint32(HRDLMagic))
	binary.Write(ws, binary.LittleEndian, size)

	digest := SumHRDL()
	ws = io.MultiWriter(ws, digest)
	if err := encodeVMUHeader(ws, p.VMUHeader); err != nil {
		return err
	}
	i, ok := p.IDH.(*panda.IDHv2)
	if !ok {
		return fmt.Errorf("invalid IDH version")
	}
	binary.Write(ws, binary.LittleEndian, i.Properties)
	binary.Write(ws, binary.LittleEndian, i.Sequence)
	binary.Write(ws, binary.LittleEndian, i.Originator)
	binary.Write(ws, binary.LittleEndian, i.Acquisition)
	binary.Write(ws, binary.LittleEndian, i.Auxiliary)
	binary.Write(ws, binary.LittleEndian, i.Id)
	binary.Write(ws, binary.LittleEndian, i.Type)
	binary.Write(ws, binary.LittleEndian, i.Pixels)
	binary.Write(ws, binary.LittleEndian, i.Region)
	binary.Write(ws, binary.LittleEndian, i.Dropping)
	binary.Write(ws, binary.LittleEndian, i.Scaling)
	binary.Write(ws, binary.LittleEndian, i.Ratio)
	ws.Write(i.Info[:])
	ws.Write(body)

	sum := digest.Sum32()
	binary.Write(ws, binary.LittleEndian, sum)

	return nil
}
