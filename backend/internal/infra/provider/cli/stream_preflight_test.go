package cli

import (
	"io"
	"strings"
	"testing"
)

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
