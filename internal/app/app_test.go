package app_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/nori-kamiya/xordpress/internal/app"
	"github.com/nori-kamiya/xordpress/internal/config"
)

// freeAddr finds an available TCP port on localhost and returns an address
// string suitable for use as cfg.Addr.
func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return addr
}

func TestRun_ServesRootAndShutsDownOnContextCancel(t *testing.T) {
	dataDir := t.TempDir()
	addr := freeAddr(t)

	cfg := config.Config{
		Addr:    addr,
		DataDir: dataDir,
		BaseURL: fmt.Sprintf("http://%s", addr),
	}

	ctx, cancel := context.WithCancel(context.Background())

	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx, cfg)
	}()

	url := fmt.Sprintf("http://%s/", addr)
	var resp *http.Response
	var lastErr error
	for i := 0; i < 50; i++ {
		resp, lastErr = http.Get(url)
		if lastErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("GET %s failed after retries: %v", url, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not shut down within 5s of context cancellation")
	}
}
