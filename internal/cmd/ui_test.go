package cmd

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chuccv/m2-docker/internal/run"
)

type slowRunner struct {
	mu       sync.Mutex
	inner    run.Runner
	inflight int32
	max      int32
}

func (s *slowRunner) Run(name string, args ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Run(name, args...)
}

func (s *slowRunner) Stream(name string, args ...string) error {
	n := atomic.AddInt32(&s.inflight, 1)
	for {
		m := atomic.LoadInt32(&s.max)
		if n <= m || atomic.CompareAndSwapInt32(&s.max, m, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	atomic.AddInt32(&s.inflight, -1)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.inner.Run(name, args...)
	return err
}

func (s *slowRunner) Pipe(stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	return s.inner.Pipe(stdin, stdout, name, args...)
}

func TestUIActionsSerialize(t *testing.T) {
	e, f, _ := upEnv(t)
	sr := &slowRunner{inner: f}
	e.runner = sr
	a := newUIActions(e)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Stop("ee249-qa")
		}()
	}
	wg.Wait()
	if sr.max != 1 {
		t.Fatalf("max concurrent %d", sr.max)
	}
}
