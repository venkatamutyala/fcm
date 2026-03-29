package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Reader wraps an io.Reader and prints download progress to stderr.
type Reader struct {
	r         io.Reader
	total     int64
	read      int64
	start     time.Time
	mu        sync.Mutex
	done      chan struct{}
	lastRead  int64
}

// NewReader creates a progress reader. total is the expected size in bytes
// (from Content-Length); pass 0 if unknown.
func NewReader(r io.Reader, total int64) *Reader {
	pr := &Reader{
		r:     r,
		total: total,
		start: time.Now(),
		done:  make(chan struct{}),
	}
	go pr.printer()
	return pr
}

func (pr *Reader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.mu.Lock()
	pr.read += int64(n)
	pr.mu.Unlock()
	return n, err
}

func (pr *Reader) printer() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-pr.done:
			return
		case <-ticker.C:
			pr.mu.Lock()
			read := pr.read
			pr.mu.Unlock()
			pr.printLine(read)
			pr.lastRead = read
		}
	}
}

func (pr *Reader) printLine(read int64) {
	readMB := read / (1024 * 1024)
	elapsed := time.Since(pr.start).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(read) / elapsed / (1024 * 1024)
	}

	if pr.total > 0 {
		totalMB := pr.total / (1024 * 1024)
		fmt.Fprintf(os.Stderr, "\rDownloading... %d/%d MB  %.1f MB/s", readMB, totalMB, speed)
	} else {
		fmt.Fprintf(os.Stderr, "\rDownloading... %d MB  %.1f MB/s", readMB, speed)
	}
}

// Finish stops the progress printer and prints the final summary line.
func (pr *Reader) Finish() {
	close(pr.done)
	pr.mu.Lock()
	read := pr.read
	pr.mu.Unlock()

	elapsed := time.Since(pr.start)
	readMB := read / (1024 * 1024)
	fmt.Fprintf(os.Stderr, "\rDownloaded %d MB in %ds\n", readMB, int(elapsed.Seconds()))
}
