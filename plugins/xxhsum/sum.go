package main

import (
	"fmt"
	"os"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/panda"
	"github.com/midbel/toml"
	"github.com/midbel/xxh"
)

func New(cfg string) (hadock.Module, error) {
	c := struct {
		Seed uint
		Alg  uint
	}{}
	r, err := os.Open(cfg)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return nil, err
	}
	var m hadock.Module
	switch c.Alg {
	case 32:
		d := digest32{
			seed: uint32(c.Seed),
			hash: xxh.Sum32,
		}
		m = d
	case 64:
		d := digest64{
			seed: uint64(c.Seed),
			hash: xxh.Sum64,
		}
		m = d
	default:
		return nil, fmt.Errorf("unsupported algorithm: %d", c.Alg)
	}
	return m, nil
}

type digest64 struct {
	seed uint64
	hash func([]byte, uint64) uint64
}

func (d digest64) Process(_ uint8, p panda.HRPacket) error {
	bs, err := p.Bytes()
	if err != nil {
		return err
	}
	if len(bs) == 0 {
		return fmt.Errorf("empy payload")
	}
	sum := d.hash(bs, d.seed)
	fmt.Printf("%s | %8d | %08x\n", p.Timestamp().Format(time.RFC3339), p.Sequence(), sum)
	return nil
}

type digest32 struct {
	seed uint32
	hash func([]byte, uint32) uint32
}

func (d digest32) Process(_ uint8, p panda.HRPacket) error {
	bs, err := p.Bytes()
	if err != nil {
		return err
	}
	if len(bs) == 0 {
		return fmt.Errorf("empy payload")
	}
	sum := d.hash(bs, d.seed)
	fmt.Printf("%s | %8d | %08x\n", p.Timestamp().Format(time.RFC3339), p.Sequence(), sum)
	return nil
}
