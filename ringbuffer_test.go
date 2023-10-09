package ringbuffer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"io"
	"math/rand"
	"testing"
)

const (
	datalen = 128 << 20
	bufsize = 64 << 10
)

var rb, _ = io.ReadAll(io.LimitReader(rand.New(rand.NewSource(0)), datalen))

func Test(t *testing.T) {
	r := bytes.NewReader(rb)

	hasher := sha256.New()
	hasher.Write(rb)
	sum1 := hasher.Sum(nil)

	hasher.Reset()

	rbuf := NewReaderSize(r, bufsize)
	buf := make([]byte, bufsize)
	for {
		n, err := rbuf.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf(`chunker error: %s`, err)
		}
		hasher.Write(buf[:n])
		if err == io.EOF {
			break
		}
	}
	sum2 := hasher.Sum(nil)

	if !bytes.Equal(sum1, sum2) {
		t.Fatalf(`ringbuffer produces incorrect output`)
	}
}

func Benchmark_IOReader(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	buf := make([]byte, bufsize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for {
			n, err := r.Read(buf)
			if err != nil && err != io.EOF {
				b.Fatalf(`io error: %s`, err)
			}
			_ = buf[:n]
			if err == io.EOF {
				break
			}
		}
		r.Reset(rb)
	}
}

func Benchmark_BufIOReader(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	buf := make([]byte, bufsize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rd := bufio.NewReaderSize(r, bufsize)
		for {
			n, err := rd.Read(buf)
			if err != nil && err != io.EOF {
				b.Fatalf(`bufio error: %s`, err)
			}
			_ = buf[:n]
			if err == io.EOF {
				break
			}
		}
		r.Reset(rb)
	}
}

func Benchmark_PlakarLabs_RingbufferReader(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	buf := make([]byte, bufsize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rd := NewReaderSize(r, bufsize)
		for {
			n, err := rd.Read(buf)
			if err != nil && err != io.EOF {
				b.Fatalf(`ringbuffer error: %s`, err)
			}
			_ = buf[:n]
			if err == io.EOF {
				break
			}
		}
		r.Reset(rb)
	}
}
