// Package app is the composition root (design.md §2): it is the only
// package permitted to import storage, content, auth, theme, httpapi,
// adminapi, and adminui together, and wires them into the running server
// described in design.md §5.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/adminui"
	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/config"
	"github.com/nori-kamiya/xordpress/internal/httpapi"
	"github.com/nori-kamiya/xordpress/internal/storage"
	"github.com/nori-kamiya/xordpress/internal/theme"
)

// Run opens the database, runs migrations, wires the HTTP routing (design.md
// §5), and serves until ctx is cancelled, at which point the server is
// gracefully shut down.
func Run(ctx context.Context, cfg config.Config) error {
	db, err := storage.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("app: open db: %w", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		return fmt.Errorf("app: migrate: %w", err)
	}

	renderer := theme.New(cfg.DataDir)

	var setupToken string
	userCount, err := storage.CountUsers(db)
	if err != nil {
		return fmt.Errorf("app: count users: %w", err)
	}
	if userCount == 0 {
		setupToken, err = auth.GenerateSetupToken()
		if err != nil {
			return fmt.Errorf("app: generate setup token: %w", err)
		}
		fmt.Printf("setup token: %s\n", setupToken)
	}

	mux := http.NewServeMux()

	mux.Handle("POST /api/setup", adminapi.Setup(db, db, setupToken))
	mux.Handle("POST /api/session", adminapi.Login(db, db, cfg.BaseURL))
	mux.Handle("/api/", auth.CSRFMiddleware(cfg.BaseURL, auth.AuthMiddleware(db, adminapi.Handler(db, db))))

	mux.Handle("/admin/", adminui.Handler())
	mux.Handle("GET /assets/", httpapi.AssetsHandler(renderer))
	mux.HandleFunc("/", httpapi.Handler(db, renderer).ServeHTTP)

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("app: listen: %w", err)
	}

	fmt.Printf("listening on %s\n", ln.Addr().String())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("app: serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		if err := server.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("app: shutdown: %w", err)
		}
		return nil
	}
}
