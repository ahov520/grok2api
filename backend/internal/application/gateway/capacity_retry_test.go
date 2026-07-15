package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

func TestHTTPUpstreamFailureClassifiesModelCapacity(t *testing.T) {
	failure := newHTTPUpstreamFailure(
		http.StatusServiceUnavailable,
		[]byte(`{"error":{"code":"model_capacity_exceeded","type":"server_error","message":"The model is currently at capacity due to high demand."}}`),
		1,
		"account",
	)
	if !failure.ModelCapacity || failure.AccountScoped {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestWaitForModelCapacityRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if waitForModelCapacityRetry(ctx, 0, 5*time.Second) {
		t.Fatal("canceled retry wait returned success")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled wait took %s", elapsed)
	}
}

func TestPromoteGenericBuildForbiddenToCredentialFailure(t *testing.T) {
	failure := newHTTPUpstreamFailure(
		http.StatusForbidden,
		[]byte(`{"error":{"code":"upstream_error","type":"server_error","message":"Upstream request failed"}}`),
		1,
		"account",
	)
	promoteBuildForbiddenCredentialFailure("grok_build", http.StatusForbidden, failure)
	if !failure.AccountScoped || !failure.CredentialRejected {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestFirstEventTimeoutIsAccountScoped(t *testing.T) {
	failure := newTransportUpstreamFailure(provider.ErrResponseFirstEventTimeout, 1, "account")
	if !failure.AccountScoped || failure.HTTPStatus != http.StatusGatewayTimeout || failure.Code != "upstream_first_event_timeout" {
		t.Fatalf("failure = %#v", failure)
	}
}
