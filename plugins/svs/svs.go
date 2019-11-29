package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/hadock/storage"
	"github.com/busoc/panda"
	"github.com/midbel/toml"
)

const origin = "90"

type UPI [32]byte

func (u UPI) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	xs := u[:]
	xs = bytes.Trim(xs, "\x00")
	return e.EncodeElement(string(xs), xml.StartElement{Name: xml.Name{Local: "upi"}})
}

type metadata struct {
	Magic       uint8         `xml:"-"`
	Acquisition time.Duration `xml:"acquisition-time"`
	Sequence    uint32        `xml:"originator-seq-no"`
	Auxiliary   time.Duration `xml:"auxiliary-time"`
	Source      uint8         `xml:"originator-id"`
	X           uint16        `xml:"source-x-size"`
	Y           uint16        `xml:"source-y-size"`
	Format      uint8         `xml:"format"`
	Drop        uint16        `xml:"fdrp"`
	OffsetX     uint16        `xml:"roi-x-offset"`
	SizeX       uint16        `xml:"roi-x-size"`
	OffsetY     uint16        `xml:"roi-y-offset"`
	SizeY       uint16        `xml:"roi-y-size"`
	ScaleX      uint16        `xml:"scale-x-size"`
	ScaleY      uint16        `xml:"scale-y-size"`
	Ratio       uint8         `xml:"scale-far"`
	UPI         UPI           `xml:"user-packet-info"`
}

type converter struct {
	dir storage.Directory
}

func New(f string) (hadock.Module, error) {
	r, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	c := struct {
		Datadir  string   `toml:"datadir"`
		Epoch    string   `toml:"time"`
		Interval int      `toml:"interval"`
		Levels   []string `toml:"levels"`
	}{}
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return nil, err
	}
	conv := converter{
		dir: storage.NewDirectory(c.Datadir, c.Epoch, c.Levels, c.Interval),
	}
	return &conv, nil
}

func (c *converter) Process(i uint8, p panda.HRPacket) error {
	if p.Version() != panda.VMUProtocol2 || p.Origin() != origin {
		return nil
	}
	if p.Sequence() == 1 {
		return c.processIntro(i, p)
	}
	dir, err := c.dir.Prepare(i, p)
	if err != nil {
		return err
	}
	file := filepath.Join(dir, p.Filename())

	rs := bytes.NewReader(p.Payload())
	if err := c.processMeta(file, rs); err != nil {
		return err
	}
	return c.processData(file, rs)
}

func (c *converter) processData(file string, rs *bytes.Reader) error {
	w, err := os.Create(file + ".csv")
	if err != nil {
		return err
	}
	defer w.Close()

	ws := csv.NewWriter(w)

	b, err := rs.ReadByte()
	if err != nil {
		return err
	}
	vs := make([]string, int(b)+1)
	vs[0] = "t"
	for j := 0; j < int(b); j++ {
		var v uint16
		if err := binary.Read(rs, binary.LittleEndian, &v); err != nil {
			return err
		}
		vs[j+1] = fmt.Sprintf("g2(t, %d)", v)
	}
	ws.Write(vs)
	for i := 0; rs.Len() > 0; i++ {
		vs[0] = strconv.Itoa(i)
		for j := 0; j < int(b); j++ {
			var v float32
			if err := binary.Read(rs, binary.LittleEndian, &v); err != nil {
				return err
			}
			vs[j+1] = strconv.FormatFloat(float64(v), 'f', -1, 32)
		}
		ws.Write(vs)
	}

	ws.Flush()
	return ws.Error()
}

func (c *converter) processMeta(file string, r io.Reader) error {
	var info metadata
	if err := binary.Read(r, binary.LittleEndian, &info); err != nil {
		return err
	}
	w, err := os.Create(file + ".xml")
	if err != nil {
		return err
	}
	defer w.Close()

	e := xml.NewEncoder(w)
	e.Indent("", "\t")
	return e.Encode(info)
}

func (c *converter) processIntro(i uint8, p panda.HRPacket) error {
	dir, err := c.dir.Prepare(i, p)
	if err != nil {
		return err
	}
	raw := p.Payload()
	file := filepath.Join(dir, p.Filename()+".ini")
	return ioutil.WriteFile(file, raw[:len(raw)-4], 0755)
}
