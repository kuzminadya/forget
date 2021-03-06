package forget

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

const (
	cacheSize = 1 << 30
	chunkSize = 1 << 10
)

type cacheIFace interface {
	Get(string, string) (io.ReadCloser, bool)
	Set(string, string, time.Duration) (io.WriteCloser, bool)
	Close()
}

type forgetCache struct {
	cache *CacheSpaces
}

type buffer struct {
	buf *bytes.Buffer
}

type baselineMap map[string]*buffer

func (f forgetCache) Get(keyspace, key string) (io.ReadCloser, bool) {
	return f.cache.Get(keyspace, key)
}

func (f forgetCache) Set(keyspace, key string, ttl time.Duration) (io.WriteCloser, bool) {
	return f.cache.Set(keyspace, key, ttl)
}

func (f forgetCache) Close() { f.cache.Close() }

func (b *buffer) Read(p []byte) (int, error)  { return b.buf.Read(p) }
func (b *buffer) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *buffer) Close() error                { return nil }

func newBaselineMap(Options) cacheIFace {
	return make(baselineMap)
}

func (m baselineMap) Get(_, key string) (io.ReadCloser, bool) {
	b, ok := m[key]
	return b, ok
}

func (m baselineMap) Set(_, key string, _ time.Duration) (io.WriteCloser, bool) {
	b := &buffer{buf: bytes.NewBuffer(nil)}
	m[key] = b
	return b, true
}

func (m baselineMap) Close() {}

func createCache(itemCount int, o Options, create func(Options) cacheIFace) cacheIFace {
	c := create(o)
	for j := 0; j < itemCount; j++ {
		w, ok := c.Set("", randomKey(), time.Hour)
		if !ok {
			panic("failed to set test data")
		}

		if _, err := io.Copy(w, bytes.NewBuffer(randomData())); err != nil {
			panic(err)
		}
	}

	return c
}

func runN(execute func(cacheIFace)) func(cacheIFace, int) {
	return func(c cacheIFace, n int) {
		for i := 0; i < n; i++ {
			execute(c)
		}
	}
}

func runConcurrent(c cacheIFace, total, concurrent int, run func(cacheIFace, int)) {
	n := total / concurrent
	if total%concurrent != 0 {
		n++
	}

	var wg sync.WaitGroup
	for total > 0 {
		if total-n < 0 {
			n = total
		}

		wg.Add(1)
		go func(n int) {
			run(c, n)
			wg.Done()
		}(n)
		total -= n
	}

	wg.Wait()
}

func benchmark(b *testing.B, parallel, itemCount, concurrent int, create func(Options) cacheIFace, execute func(cacheIFace)) {
	if !randomInitialized {
		initRandom()
	}

	c := createCache(itemCount, Options{
		CacheSize:       cacheSize,
		ChunkSize:       chunkSize,
		maxSegmentCount: parallel,
	}, create)
	defer c.Close()

	b.ResetTimer()
	runConcurrent(c, b.N, concurrent, runN(execute))
}

func executeKey(execute func(cacheIFace, string)) func(cacheIFace) {
	return func(c cacheIFace) {
		execute(c, randomKey())
	}
}

func executeGet(c cacheIFace, key string) {
	if r, ok := c.Get("", key); ok {
		r.Close()
	}
}

func executeSet(c cacheIFace, key string) {
	if w, ok := c.Set("", key, time.Hour); !ok {
		panic("failed to set key")
	} else {
		w.Close()
	}
}

func newForget(o Options) cacheIFace { return forgetCache{NewCacheSpaces(o)} }

func benchmarkRange(b *testing.B, parallel, concurrent int, create func(Options) cacheIFace, execute func(cacheIFace)) {
	for _, itemCount := range []int{0, 10, 1000, 100000} {
		b.Run(fmt.Sprintf("item count = %d", itemCount), func(b *testing.B) {
			benchmark(b, parallel, itemCount, concurrent, create, execute)
		})
	}
}

func BenchmarkGet_Baseline_1(b *testing.B) {
	benchmarkRange(b, 1, 1, newBaselineMap, executeKey(executeGet))
}

func BenchmarkSet_Baseline_1(b *testing.B) {
	benchmarkRange(b, 1, 1, newBaselineMap, executeKey(executeSet))
}

func BenchmarkGet_NoConcurrency_1(b *testing.B) {
	benchmarkRange(b, 1, 1, newForget, executeKey(executeGet))
}

func BenchmarkSet_NoConcurrency_1(b *testing.B) {
	benchmarkRange(b, 1, 1, newForget, executeKey(executeSet))
}

func BenchmarkGet_TwoCores_100000(b *testing.B) {
	benchmarkRange(b, 2, 100000, newForget, executeKey(executeGet))
}

func BenchmarkSet_TwoCores_100000(b *testing.B) {
	benchmarkRange(b, 2, 100000, newForget, executeKey(executeSet))
}

func BenchmarkGet_AllCores_100000(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 100000, newForget, executeKey(executeGet))
}

func BenchmarkSet_AllCores_100000(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 100000, newForget, executeKey(executeSet))
}

func BenchmarkGet_AllCores_1000(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 1000, newForget, executeKey(executeGet))
}

func BenchmarkSet_AllCores_1000(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 1000, newForget, executeKey(executeSet))
}

func BenchmarkGet_AllCores_10(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 10, newForget, executeKey(executeGet))
}

func BenchmarkSet_AllCores_10(b *testing.B) {
	benchmarkRange(b, runtime.NumCPU(), 10, newForget, executeKey(executeSet))
}
