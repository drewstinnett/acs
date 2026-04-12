package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/drewstinnett/acs/internal/store"
)

//go:embed static
var embeddedStatic embed.FS

// ListenAndServe starts the HTTP server on addr (e.g. "127.0.0.1:8080").
func ListenAndServe(addr string, db *store.DB, dataDir string, log *slog.Logger) error {
	srv := NewServer(db, dataDir, log)

	// Serve static files from embedded FS.
	staticSub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}
	srv.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	log.Info("starting web server", "addr", addr)
	return http.ListenAndServe(addr, srv)
}
