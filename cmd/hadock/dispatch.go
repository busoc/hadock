package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"

	"github.com/midbel/cli"
	"github.com/midbel/toml"
)

func runDispatch(cmd *cli.Command, args []string) error {
	if err := cmd.Flag.Parse(args); err != nil {
		return err
	}
	r, err := os.Open(cmd.Flag.Arg(0))
	if err != nil {
		return err
	}
	defer r.Close()
	c := struct {
		In  string `toml:"datadir"`
		Out string `toml:""`
	}{}
	if err := toml.NewDecoder(r).Decode(&c); err != nil {
		return err
	}

	bs := make([]byte, 64<<10)
	return filepath.Walk(c.In, func(p string, i os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if i.IsDir() {
			return nil
		}
		r, err := os.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()

		s := bufio.NewScanner(r)
		s.Buffer(bs, 8<<20)
		s.Split(scanPackets)
		for s.Scan() {
			meta := struct {
			}{}
			rs := bytes.NewReader(s.Bytes())
			if err := binary.Read(rs, binary.BigEndian, &meta); err != nil {
				return err
			}
		}
		return s.Err()
	})
}

func scanPackets(bs []byte, ateof bool) (int, []byte, error) {
	if ateof {
		return len(bs), bs, bufio.ErrFinalToken
	}
	if len(bs) < 4 {
		return 0, nil, nil
	}
	size := int(binary.BigEndian.Uint32(bs)) + 4
	if len(bs) < size {
		return 0, nil, nil
	}
	xs := make([]byte, size-4)
	copy(xs, bs[4:])
	return size, xs, nil
}
