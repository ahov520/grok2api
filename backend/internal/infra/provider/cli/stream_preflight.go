package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

const maxResponseStreamPreflightBytes = 1 << 20
const responseStreamFirstEventTimeout = 15 * time.Second

type prefetchedReadCloser struct {
	io.Reader
	io.Closer
}

func preflightResponseStream(source io.ReadCloser) (io.ReadCloser, []byte, bool, error) {
	return preflightResponseStreamWithTimeout(source, responseStreamFirstEventTimeout)
}

type responseStreamPreflightResult struct {
	body      io.ReadCloser
	failure   []byte
	immediate bool
	err       error
}

func preflightResponseStreamWithTimeout(source io.ReadCloser, timeout time.Duration) (io.ReadCloser, []byte, bool, error) {
	result := make(chan responseStreamPreflightResult, 1)
	go func() {
		body, failure, immediate, err := readResponseStreamPreflight(source)
		result <- responseStreamPreflightResult{body: body, failure: failure, immediate: immediate, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case value := <-result:
		return value.body, value.failure, value.immediate, value.err
	case <-timer.C:
		_ = source.Close()
		return nil, nil, false, provider.ErrResponseFirstEventTimeout
	}
}

func readResponseStreamPreflight(source io.ReadCloser) (io.ReadCloser, []byte, bool, error) {
	reader := bufio.NewReaderSize(source, 64<<10)
	var prefetched bytes.Buffer
	for prefetched.Len() <= maxResponseStreamPreflightBytes {
		line, err := reader.ReadString('\n')
		if line != "" {
			prefetched.WriteString(line)
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			}
			if trimmed == "[DONE]" {
				return &prefetchedReadCloser{Reader: io.MultiReader(bytes.NewReader(prefetched.Bytes()), reader), Closer: source}, nil, false, nil
			}
			if strings.HasPrefix(trimmed, "{") {
				data := []byte(trimmed)
				if isImmediateResponseError(data) {
					_ = source.Close()
					return nil, data, true, nil
				}
				return &prefetchedReadCloser{Reader: io.MultiReader(bytes.NewReader(prefetched.Bytes()), reader), Closer: source}, nil, false, nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return &prefetchedReadCloser{Reader: bytes.NewReader(prefetched.Bytes()), Closer: source}, nil, false, nil
			}
			_ = source.Close()
			return nil, nil, false, err
		}
	}
	_ = source.Close()
	return nil, nil, false, errors.New("response stream preflight exceeded 1 MiB")
}

func isImmediateResponseError(data []byte) bool {
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return false
	}
	if value, exists := root["error"]; exists && value != nil {
		return true
	}
	if response, ok := root["response"].(map[string]any); ok {
		if value, exists := response["error"]; exists && value != nil {
			return true
		}
	}
	typeName, _ := root["type"].(string)
	return typeName == "error" || typeName == "response.failed"
}

func responseStreamErrorStatus(data []byte) int {
	normalized := strings.ToLower(string(data))
	if strings.Contains(normalized, "rate limit") || strings.Contains(normalized, "rate_limit") || strings.Contains(normalized, "too many requests") {
		return http.StatusTooManyRequests
	}
	if strings.Contains(normalized, "capacity") || strings.Contains(normalized, "high demand") || strings.Contains(normalized, "overloaded") {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}
