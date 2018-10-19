package main

import (
  "flag"
  "time"
  "log"
  "path/filepath"
  "os"
  "fmt"
  "bytes"
  "io"
  "encoding/binary"
  "encoding/csv"
  "strconv"
)

var FCC = []byte("MMA ")

var GPS = time.Date(1980, 1, 6, 0, 0, 0, 0, time.UTC)

func main() {
  datadir := flag.String("w", os.TempDir(), "working directory")
  flag.Parse()

  for _, p := range flag.Args() {
    if err := process(p, *datadir); err != nil {
      log.Printf("%s: %s", p, err)
    }
  }
}

func process(file, datadir string) error {
  r, err := os.Open(file)
  if err != nil {
    return err
  }
  defer r.Close()

  when, err := readPreamble(r)
  if err != nil {
    return err
  }
  dir, base := filepath.Split(file)
  dir = filepath.Join(datadir, dir)
  if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
    return err
  }
  w, err := os.Create(filepath.Join(dir, base))
  if err != nil {
    return err
  }
  defer w.Close()

  return writeScience(w, r, when)
}

func writeScience(w io.Writer, r io.Reader, t time.Time) error {
  c := csv.NewWriter(w)
	for i := 1; ; i++ {
    rs := make([]string, 35)
    rs[0] = t.Format(time.RFC3339)
		rs[1] = strconv.FormatInt(int64(i%16), 10)
		rs[2] = "16" // TBD
    for i := 3; i < len(rs); i++ {
      var v uint16
      if err := binary.Read(r, binary.BigEndian, &v); err != nil {
        if err == io.EOF {
          c.Flush()
        	return c.Error()
        }
				return err
			}
      rs[i] = strconv.FormatUint(uint64(v), 10)
    }
    if err := c.Write(rs); err != nil {
			return err
		}
  }
}

func readPreamble(r io.Reader) (time.Time, error) {
  var t time.Time

  magic := make([]byte, len(FCC))
  if _, err := io.ReadFull(r, magic); err != nil {
    return t, err
  }
  if !bytes.Equal(magic, FCC) {
    return t, fmt.Errorf("invalid FCC (expected: %x, got: %x)", FCC, magic)
  }
  var (
    seq uint32
    when int64
  )
  binary.Read(r, binary.BigEndian, &seq)
  binary.Read(r, binary.BigEndian, &when)

  return GPS.Add(time.Duration(when) * time.Nanosecond), nil
}
