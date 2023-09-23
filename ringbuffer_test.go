package ringbuffer

import (
	"bufio"
	"bytes"
	"io"
	"math/rand"
	"testing"
)

const (
	minsize = 2 << 10
	avgsize = 10 << 10
	maxsize = 64 << 10
	norm    = 0
	datalen = 128 << 20
)

type writerFunc func([]byte) (int, error)

func (fn writerFunc) Write(p []byte) (int, error) {
	return fn(p)
}

var rb, _ = io.ReadAll(io.LimitReader(rand.New(rand.NewSource(0)), datalen))

func Test(t *testing.T) {
}

func Benchmark_IOReader(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	buf := make([]byte, 8<<10)
	b.ResetTimer()
	nchunks := 0
	for i := 0; i < b.N; i++ {
		_, _ = r.Read(buf)
		nchunks++
		r.Reset(rb)
	}
	b.ReportMetric(float64(nchunks)/float64(b.N), "chunks")
}

func Benchmark_BufIOReader(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	buf := make([]byte, 8<<10)

	rd := bufio.NewReader(r)
	b.ResetTimer()
	nchunks := 0
	for i := 0; i < b.N; i++ {
		_, _ = rd.Read(buf)
		nchunks++
		r.Reset(rb)
	}
	b.ReportMetric(float64(nchunks)/float64(b.N), "chunks")
}

func Benchmark(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))

	b.ResetTimer()
	nchunks := 0
	rd := NewReaderSize(r, 16<<10)
	for i := 0; i < b.N; i++ {
		_, _ = rd.Peek(8 << 10)
		_, _ = rd.Discard(8 << 10)
		nchunks++
		r.Reset(rb)
	}
	b.ReportMetric(float64(nchunks)/float64(b.N), "chunks")
}
