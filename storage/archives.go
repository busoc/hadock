package storage

import (
  "fmt"
  "sort"

  "github.com/busoc/panda"
)

type tarstore struct {
	sources []string
}

type zipstore struct {
}

func (z *zipstore) Store(i uint8, p panda.HRPacket) error {
	return nil
}

func (t *tarstore) Store(i uint8, p panda.HRPacket) error {
	ix := sort.SearchStrings(t.sources, p.Origin())
	if ix == len(t.sources) || t.sources[ix] != p.Origin() {
		return nil
	}
	return nil
}

func NewArchiveStorage(f string) (Storage, error) {
	var s Storage
	switch f {
	case "tar", "archive":
		s = &tarstore{}
	case "zip":
		s = &zipstore{}
	default:
		return nil, fmt.Errorf("invalid %s", f)
	}
	return s, nil
}
