package main

import (
  "bytes"
  "fmt"
  "flag"
  "log"
  "io"
  "os"
)

var FCC = []byte("MMA ")

func main() {
  flag.Parse()
  for _, p := range flag.Args() {
    if err := process(p); err != nil {
      log.Printf("%s: %s", p, err)
    }
  }
}

const bytesToSkip = 16

func process(p string) error {
  r, err := os.Open(p)
  if err != nil {
    return err
  }
  defer r.Close()
  magic := make([]byte, 4)
  if _, err := r.Read(magic); err != nil {
    return err
  }
  if !bytes.Equal(magic, FCC) {
    return fmt.Errorf("expected magic %s (found: %s)", FCC, magic)
  }
  if _, err := r.Seek(12, io.SeekCurrent); err != nil {
    return err
  }
  logger := log.New(os.Stdout, "", 0)
  var w bytes.Buffer
  for {
    bs := make([]byte, 64)
    if _, err := io.ReadFull(r, bs); err != nil {
      if err == io.EOF {
        break
      }
      return err
    }
    bs = bytes.Trim(bs[2:], "\x00")
    for i := 0; i < len(bs); i += 2 {
      w.WriteByte(bs[i])
      w.WriteByte(bs[i+1])
    }
  }
  if w.Len() > 0 {
    logger.Println(w.String())
  }
  return nil
}
