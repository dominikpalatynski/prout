package githubapp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientDoStreamUsesStreamingHTTPClient(t *testing.T) {
	httpClientErr := errors.New("timed client should not be used")

	client := &Client{
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, httpClientErr
		})},
		streamHTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("payload")),
			}, nil
		})},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/archive.tar.gz", http.NoBody)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}

	body, err := client.doStream(req)
	if err != nil {
		t.Fatalf("doStream() error = %v", err)
	}
	defer body.Close()

	payload, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if string(payload) != "payload" {
		t.Fatalf("payload = %q, want %q", string(payload), "payload")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
