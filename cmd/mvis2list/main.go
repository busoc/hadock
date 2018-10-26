package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

var FCC = []byte("MMA ")

func main() {
	file := flag.String("f", "", "file")
	erase := flag.Bool("x", false, "erase")
	flag.Parse()
	ps := flag.Args()
	if len(ps) == 0 {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			ps = append(ps, s.Text())
		}
		if err := s.Err(); err != nil {
			log.Fatalln(err)
		}
	}
	if *erase && *file != "" {
		err := os.Remove(*file)
		if err != nil && !os.IsNotExist(err) {
			log.Fatalln(err)
		}
	}
	dir := filepath.Dir(*file)
	if err := os.MkdirAll(dir, 0755); dir != "" && err != nil && !os.IsExist(err) {
		log.Fatalln(err)
	}
	var w io.Writer
	switch f, err := os.OpenFile(*file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); {
	case *file == "":
		w = os.Stdout
	case *file != "" && err == nil:
		defer f.Close()
		w = f
	default:
		log.Fatalln(err)
	}
	for _, p := range ps {
		if err := process(w, p); err != nil {
			log.Printf("%s: %s", p, err)
		}
	}
}

const bytesToSkip = 16

func process(ws io.Writer, p string) error {
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
	var w bytes.Buffer
	for {
		bs := make([]byte, 64)
		if n, err := io.ReadFull(r, bs); err != nil {
			if err == io.EOF {
				break
			}
			return err
		} else {
			bs = bs[:n]
		}
		bs = bytes.Trim(bs[2:], "\x00")
		for i := 0; i < len(bs); i += 2 {
			w.WriteByte(bs[i])
			if j := i + 1; j < len(bs) {
				w.WriteByte(bs[j])
			}
		}
	}
	if w.Len() > 0 {
		io.WriteString(ws, w.String())
	}
	return nil
}
