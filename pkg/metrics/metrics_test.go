package metrics_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/pkg/metrics"
)

func freeAddr(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	return addr
}

func TestServer(t *testing.T) {
	t.Parallel()

	reg := metrics.NewRegistry()
	calls := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_calls_total", Help: "Calls."})
	reg.MustRegister(calls)
	calls.Add(3)

	addr := freeAddr(t)
	srv := metrics.NewServer(addr, reg, slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	body := scrape(t, "http://"+addr+metrics.Path)
	for _, want := range []string{"test_calls_total 3", "go_goroutines", "process_cpu_seconds_total"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics have no %q", want)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestServer_PortBusy(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = lis.Close() }()

	srv := metrics.NewServer(lis.Addr().String(), metrics.NewRegistry(), slog.New(slog.DiscardHandler))
	if err := srv.Run(t.Context()); err == nil {
		t.Error("Run on a busy port = nil, want an error")
	}
}

// scrape retries until the server listens.
func scrape(t *testing.T, url string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("io.ReadAll: %v", err)
			}
			return string(body)
		}

		if time.Now().After(deadline) {
			t.Fatalf("http.Get: %v", err)
		}
		runtime.Gosched()
	}
}
