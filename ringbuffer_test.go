package ringbuffer

import (
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

func Benchmark(b *testing.B) {
	r := bytes.NewReader(rb)
	b.SetBytes(int64(r.Len()))
	b.ResetTimer()
	//nchunks := 0
	for i := 0; i < b.N; i++ {
		r.Reset(rb)
	}
	// b.ReportMetric(float64(nchunks)/float64(b.N), "chunks")
}
