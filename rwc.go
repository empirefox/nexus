package nexus

import (
	"io"
	"sync"
)

type ReadWriteCloser struct {
	io.ReadCloser
	w     io.Writer
	flush func()
	mu    sync.Mutex
}

func (rwc ReadWriteCloser) Write(p []byte) (n int, err error) {
	rwc.mu.Lock()
	defer rwc.mu.Unlock()

	if n, err = rwc.w.Write(p); err != nil {
		return
	}
	rwc.flush()
	return
}
