package storage

import (
	// "archive/tar"
	// "archive/zip"
	"fmt"

	"github.com/busoc/panda"
)

type tarstore struct{}

type zipstore struct{}

func (z *zipstore) Store(i uint8, p panda.HRPacket) error {
	return nil
}

func (t *tarstore) Store(i uint8, p panda.HRPacket) error {
	return nil
}

func NewArchiveStorage(o Options) (Storage, error) {
	var s Storage
	switch o.Scheme {
	case "tar", "archive":
	case "zip":
	default:
		return nil, fmt.Errorf("unsupported archive format %s", o.Scheme)
	}
	return s, nil
}
