package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMediaRoundTripPreservesSignedHostAndPeer(t *testing.T) {
	var mu sync.Mutex
	var objectData []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/media/uploads":
			if r.Header.Get("Authorization") != "Bearer sender" {
				t.Error("sender token lost")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"uploadId": "ticket", "uploadUrl": "http://minio:9000/messenger-media/object?signature=secret"})
		case "/messenger-media/object":
			if r.Host != "minio:9000" {
				t.Error("signed host changed")
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Method == "PUT" {
				objectData, _ = io.ReadAll(r.Body)
			} else {
				_, _ = w.Write(objectData)
			}
		case "/v1/media/uploads/ticket/confirm":
			_ = json.NewEncoder(w).Encode(map[string]string{"mediaId": "media"})
		case "/v1/media/media/download-url":
			if r.Header.Get("Authorization") != "Bearer receiver" {
				t.Error("download does not use peer")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"downloadUrl": "http://minio:9000/messenger-media/object?signature=secret"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := config{mediaURL: srv.URL, mediaBytes: 65536}
	client := mediaClient(strings.TrimPrefix(srv.URL, "http://"))
	defer client.CloseIdleConnections()
	bytes, err := transferMedia(t.Context(), client, c, user{token: "sender", peerToken: "receiver", chatID: "chat"}, "operation")
	if err != nil || bytes != 65536 {
		t.Fatalf("media: %d %v", bytes, err)
	}
}

func TestHTTPFailuresDoNotLeakBodiesOrURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, "private-secret")
	}))
	defer srv.Close()
	err := requestJSON(t.Context(), httpClient, "GET", srv.URL+"?token=secret", "secret", nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
