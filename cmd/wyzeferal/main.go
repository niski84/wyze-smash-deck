package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"wyze-smash-deck/internal/wyzeferal"
)

func main() {
	cfg := wyzeferal.LoadAppConfig(wyzeferal.DefaultSettingsPath())
	srv := wyzeferal.NewHTTPServer(cfg)
	srv.StartScheduler()
	defer srv.StopScheduler()

	webDir := filepath.Clean("web/wyzeferal")
	if _, err := os.Stat(webDir); err != nil {
		log.Fatalf("web UI not found at %s (run from repo root)", webDir)
	}

	addr := ":" + cfg.Port
	fmt.Printf("[BOOT] Wyze Feral Smash Deck on %s (web %s)\n", addr, webDir)
	log.Fatal(http.ListenAndServe(addr, srv.Routes(webDir)))
}
