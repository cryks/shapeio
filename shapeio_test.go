package shapeio_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cryks/shapeio"
	"github.com/dustin/go-humanize"
)

var rates = []float64{
	500 * 1024,       // 500KB/sec
	1024 * 1024,      // 1MB/sec
	10 * 1024 * 1024, // 10MB/sec
	50 * 1024 * 1024, // 50MB/sec
}

var srcs = []*bytes.Reader{
	bytes.NewReader(bytes.Repeat([]byte{0}, 64*1024)),   // 64KB
	bytes.NewReader(bytes.Repeat([]byte{1}, 256*1024)),  // 256KB
	bytes.NewReader(bytes.Repeat([]byte{2}, 1024*1024)), // 1MB
}

func ExampleReader() {
	// example for downloading http body with rate limit.
	resp, _ := http.Get("http://example.com")
	defer resp.Body.Close()

	reader := shapeio.NewReader(resp.Body)
	reader.SetRateLimit(1024 * 10) // 10KB/sec
	io.Copy(ioutil.Discard, reader)
}

func ExampleWriter() {
	// example for writing file with rate limit.
	src := bytes.NewReader(bytes.Repeat([]byte{0}, 32*1024)) // 32KB
	f, _ := os.Create("/tmp/foo")
	writer := shapeio.NewWriter(f)
	writer.SetRateLimit(1024 * 10) // 10KB/sec
	io.Copy(writer, src)
	f.Close()
}

func TestRead(t *testing.T) {
	for _, src := range srcs {
		for _, limit := range rates {
			src.Seek(0, 0)
			sio := shapeio.NewReader(src)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(ioutil.Discard, sio)
			elapsed := time.Since(start)
			if err != nil {
				t.Error("io.Copy failed", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit {
				t.Errorf("Limit %f but real rate %f", limit, realRate)
			}
			t.Logf(
				"read %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				humanize.IBytes(uint64(n)),
				elapsed,
				humanize.IBytes(uint64(realRate)),
				humanize.IBytes(uint64(limit)),
				realRate/limit*100,
			)
		}
	}
}

func TestWrite(t *testing.T) {
	for _, src := range srcs {
		for _, limit := range rates {
			src.Seek(0, 0)
			sio := shapeio.NewWriter(ioutil.Discard)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(sio, src)
			elapsed := time.Since(start)
			if err != nil {
				t.Error("io.Copy failed", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit {
				t.Errorf("Limit %f but real rate %f", limit, realRate)
			}
			t.Logf(
				"write %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				humanize.IBytes(uint64(n)),
				elapsed,
				humanize.IBytes(uint64(realRate)),
				humanize.IBytes(uint64(limit)),
				realRate/limit*100,
			)
		}
	}
}

// https://github.com/fujiwara/shapeio/issues/2
func TestConcurrentSetRateLimit(t *testing.T) {
	// run with go test -race
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	sio := shapeio.NewWriter(ioutil.Discard)

	for _, l := range rates {
		limit := l
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(50 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					sio.SetRateLimit(limit)
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, src := range srcs {
					io.Copy(sio, src)
				}
			}
		}
	}()

	time.AfterFunc(time.Second, cancel)

	wg.Wait()
}
