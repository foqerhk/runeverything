package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/foqerhk/runeverything/internal/directory"
)

func main() {
	listen := flag.String("listen", ":8790", "HTTP listen address")
	ttl := flag.Duration("ttl", directory.DefaultTTL, "relay entry TTL without heartbeat")
	allowWS := flag.Bool("allow-ws", false, "allow ws:// announces (dev only; production should use wss://)")
	flag.Parse()

	reg := directory.NewRegistry(*ttl, *allowWS)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/v1/relays", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, directory.ListResponse{Relays: reg.List()})
	})

	mux.HandleFunc("/v1/relays/announce", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req directory.AnnounceRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		ent, err := reg.Announce(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, ent)
	})

	log.SetOutput(os.Stderr)
	log.Printf("RunEverything directory listening on %s (ttl=%s allow_ws=%v)", *listen, *ttl, *allowWS)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
