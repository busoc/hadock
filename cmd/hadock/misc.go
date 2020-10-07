package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/panda"
	"github.com/midbel/cli"
)

func runMonitor(cmd *cli.Command, args []string) error {
	if err := cmd.Flag.Parse(args); err != nil {
		return err
	}
	r, err := readMessages(cmd.Flag.Args())
	if err != nil {
		return err
	}
	for {
		m, err := hadock.DecodeMessage(r)
		if err != nil {
			log.Fatalln(err)
		}
		mode := "realtime"
		if !m.Realtime {
			mode = "playback"
		}
		log.Printf("%s | %9d | %3d | %d | %18s | %9d | %12s | %6.3g | %s | %s | %s",
			m.Origin,
			m.Sequence,
			m.Instance,
			m.Channel,
			mode,
			m.Count,
			m.Elapsed,
			float64(m.Count)/m.Elapsed.Seconds(),
			//time.Unix(m.Generated, 0).Format(time.RFC3339),
			panda.AdjustGenerationTime(m.Generated).Format(time.RFC3339),
			time.Unix(m.Acquired, 0).Format(time.RFC3339),
			m.Reference,
		)
	}
}

func readMessages(gs []string) (io.Reader, error) {
	pr, pw := io.Pipe()
	for _, g := range gs {
		a, err := net.ResolveUDPAddr("udp", g)
		if err != nil {
			return nil, err
		}
		c, err := net.ListenMulticastUDP("udp", nil, a)
		if err != nil {
			return nil, err
		}
		go func(rc io.ReadCloser) {
			defer rc.Close()
			for {
				_, err := io.Copy(pw, rc)
				e, ok := err.(net.Error)
				if !ok {
					log.Println(err)
					return
				}
				if !(e.Temporary() || e.Timeout()) {
					log.Println(err)
					return
				}
			}
		}(c)
	}
	return bufio.NewReader(pr), nil
}
