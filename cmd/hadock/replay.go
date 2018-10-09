package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/busoc/hadock"
	"github.com/busoc/panda"
	"github.com/midbel/cli"
	"github.com/midbel/rustine/sum"
	"golang.org/x/time/rate"
)

func runReplay(cmd *cli.Command, args []string) error {
	rate, _ := cli.ParseSize("8M")
	cmd.Flag.Var(&rate, "r", "rate")
	// rate := cmd.Flag.Duration("r", time.Second, "rate")
	size := cmd.Flag.Int("s", 0, "chunk size")
	mode := cmd.Flag.Int("m", hadock.OPS, "mode")
	num := cmd.Flag.Int("n", 0, "count")
	vmu := cmd.Flag.Int("t", panda.VMUProtocol2, "vmu version")
	if err := cmd.Flag.Parse(args); err != nil {
		return err
	}
	c, err := Replay(cmd.Flag.Arg(0), *size, *vmu, *mode, rate)
	if err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Kill, os.Interrupt)

	n := time.Now()
	var (
		count uint64
		bytes uint64
	)
	defer func() {
		c.Close()
		log.Printf("%d packets (%.2fKB) processed in %s", count, float64(bytes)/1024, time.Since(n))
	}()
	queue := walkPaths(cmd.Flag.Args()[1:])
	for i := 0; *num <= 0 || i < *num; i++ {
		select {
		case bs, ok := <-queue:
			if !ok {
				return nil
			}
			if _, err := c.Write(bs); err != nil {
				log.Println(err)
				if err, ok := err.(net.Error); ok && !err.Temporary() {
					return nil
				}
			}
			count, bytes = count+1, bytes+uint64(len(bs))
		case <-sig:
			return nil
		}
	}
	return nil
}

type replay struct {
	net.Conn

	limiter *rate.Limiter

	size    int
	counter uint16
	version uint16
}

func Replay(a string, s, t, m int, z cli.Size) (net.Conn, error) {
	c, err := net.Dial("tcp", a)
	if err != nil {
		return nil, err
	}
	p := hadock.HadockVersion2
	if s <= 0 {
		p = hadock.HadockVersion1
	}
	r := &replay{
		Conn:    c,
		limiter: rate.NewLimiter(rate.Limit(z.Float()), int(z.Int())/10),
		size:    s,
		version: uint16(p)<<12 | uint16(t)<<8 | uint16(m),
	}
	return r, nil
}

func (r *replay) Write(bs []byte) (int, error) {
	defer func() {
		r.counter++
	}()
	v := r.limiter.ReserveN(time.Now(), len(bs))
	if !v.OK() {
		// log.Println("not allow to write")
		return 0, nil
	}
	time.Sleep(v.Delay())

	return r.writePacket(bs)
}

func (r *replay) writePacket(bs []byte) (int, error) {
	if r.size <= 0 {
		_, err := io.Copy(r.Conn, r.preparePacketV1(bs))
		return len(bs), err
	}
	for _, w := range r.preparePacketV2(bs) {
		var total int
		if c, err := io.Copy(r.Conn, w); err != nil {
			return total + int(c), err
		} else {
			total += int(c)
		}
	}
	return len(bs), nil
}

func (r *replay) preparePacketV1(bs []byte) io.Reader {
	w := new(bytes.Buffer)
	binary.Write(w, binary.BigEndian, hadock.Preamble)
	binary.Write(w, binary.BigEndian, r.version)
	binary.Write(w, binary.BigEndian, r.counter)
	binary.Write(w, binary.BigEndian, uint32(len(bs)))
	w.Write(bs)
	binary.Write(w, binary.BigEndian, sum.Sum1071(bs))

	return w
}

func (r *replay) preparePacketV2(bs []byte) []io.Reader {
	re := bytes.NewBuffer(bs)
	c := re.Len() / r.size
	rs := make([]io.Reader, 0, c)
	for i := 0; re.Len() > 0; i++ {
		vs := re.Next(r.size)
		s := sum.Sum1071(vs)

		w := new(bytes.Buffer)
		binary.Write(w, binary.BigEndian, hadock.Preamble)
		binary.Write(w, binary.BigEndian, r.version)
		binary.Write(w, binary.BigEndian, uint16(i))
		binary.Write(w, binary.BigEndian, uint16(c))
		binary.Write(w, binary.BigEndian, r.counter)
		binary.Write(w, binary.BigEndian, uint32(len(vs)))
		w.Write(vs)
		binary.Write(w, binary.BigEndian, s)

		rs = append(rs, w)
	}
	return rs
}

func walkPaths(ds []string) <-chan []byte {
	q := make(chan []byte)
	go func() {
		defer close(q)
		for _, d := range ds {
			queue, err := walk(d)
			if err != nil {
				continue
			}
			for bs := range queue {
				q <- bs
			}
		}
	}()
	return q
}

func walk(d string) (<-chan []byte, error) {
	q := make(chan []byte)
	go func() {
		defer close(q)

		buf := make([]byte, 8<<20)
		err := filepath.Walk(d, func(p string, i os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if i.IsDir() {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()

			s := bufio.NewScanner(f)
			s.Buffer(buf, len(buf))
			s.Split(scanVMUPackets)
			for s.Scan() {
				q <- s.Bytes()
			}
			return s.Err()
		})
		if err != nil {
			log.Println(err)
		}
	}()
	return q, nil
}

func scanVMUPackets(bs []byte, ateof bool) (int, []byte, error) {
	if len(bs) < 4 {
		return 0, nil, nil
	}
	s := int(binary.LittleEndian.Uint32(bs[:4]) + 4)
	if s >= len(bs) {
		return 0, nil, nil
	}
	vs := make([]byte, s-panda.HRDPHeaderLength-panda.HRDLSyncLength-4)
	copy(vs, bs[4+panda.HRDPHeaderLength+panda.HRDLSyncLength:])
	return s, vs, nil
}
