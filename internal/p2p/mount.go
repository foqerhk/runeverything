package p2p

import (
	"encoding/json"
	"net/http"
	"time"
)

// Mount registers Bitcoin-style peer exchange HTTP endpoints on mux.
func Mount(mux *http.ServeMux, store *Store, sharing bool) {
	mux.HandleFunc("/v1/peers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, store.SnapshotForGossip(sharing))
		case http.MethodPost:
			var msg AnnouncePeers
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
			if err := dec.Decode(&msg); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			now := time.Now().UTC()
			if msg.Self != "" {
				_ = store.Upsert(Peer{URL: msg.Self, LastSeen: now})
			}
			for i := range msg.Peers {
				if msg.Peers[i].LastSeen.IsZero() {
					msg.Peers[i].LastSeen = now
				}
				_ = store.Upsert(msg.Peers[i])
			}
			writeJSON(w, map[string]any{"ok": true, "peers": len(store.List())})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
