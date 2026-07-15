package cli

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

type blockingReadCloser struct {
	done chan struct{}
	once sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{done: make(chan struct{})}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	<-r.done
	return 0, io.EOF
}

func (r *blockingReadCloser) Close() error {
	r.once.Do(func() { close(r.done) })
	return nil
}

func TestPreflightResponseStreamDetectsCapacityError(t *testing.T) {
	const event = "event: error\ndata: {\"type\":\"error\",\"message\":\"The model is currently at capacity.\"}\n\n"
	body, failure, immediate, err := preflightResponseStream(io.NopCloser(strings.NewReader(event)))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil || !immediate || !strings.Contains(string(failure), "capacity") {
		t.Fatalf("body=%v immediate=%v failure=%s", body, immediate, failure)
	}
	if status := responseStreamErrorStatus(failure); status != 503 {
		t.Fatalf("status = %d", status)
	}
}

func TestPreflightResponseStreamPreservesNormalEvent(t *testing.T) {
	const event = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\ndata: [DONE]\n\n"
	body, _, immediate, err := preflightResponseStream(io.NopCloser(strings.NewReader(event)))
	if err != nil {
		t.Fatal(err)
	}
	if immediate {
		t.Fatal("normal stream reported as immediate error")
	}
	converted, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(converted) != event {
		t.Fatalf("stream changed:\n%s", converted)
	}
}

func TestPreflightResponseStreamTimesOutBeforeFirstEvent(t *testing.T) {
	source := newBlockingReadCloser()
	started := time.Now()
	_, _, _, err := preflightResponseStreamWithTimeout(source, 20*time.Millisecond)
	if !errors.Is(err, provider.ErrResponseFirstEventTimeout) {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}
