package matterroom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestMatchRoutesMatterAndBuildsInfraRequests(t *testing.T) {
	fixedNow := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		signed    bool
		deadline  time.Time
		wantQueue string
	}{
		{"unsigned waits for delivery", false, fixedNow.Add(24 * time.Hour), "awaiting-signature"},
		{"signed near deadline is prioritized", true, fixedNow.Add(36 * time.Hour), "deadline-priority"},
		{"signed later matter is standard", true, fixedNow.Add(7 * 24 * time.Hour), "standard-review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatal("missing bearer authorization")
				}
				if r.Header.Get("Idempotency-Key") == "" {
					t.Fatal("missing idempotency key")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/realtime/token/issue" {
					_, _ = w.Write([]byte(`{"ok":true,"data":{"token":"realtime-client-token"},"metadata":{}}`))
					return
				}
				if r.URL.Path == "/v1/rtc/token/issue" {
					_, _ = w.Write([]byte(`{"ok":true,"data":{"token":"rtc-client-token"},"metadata":{}}`))
					return
				}
				_, _ = w.Write([]byte(`{"ok":true,"data":{},"metadata":{}}`))
			}))
			defer server.Close()

			client := newClient(server.URL, "test-key", server.Client())
			workflow := NewWorkflow(client)
			workflow.now = func() time.Time { return fixedNow }
			got, err := workflow.Match(context.Background(), MatterIntake{
				MatterID: "M-204", ClientID: "client-17", ClientName: "Avery Chen",
				Deadline: tt.deadline, DocumentSigned: tt.signed,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Queue != tt.wantQueue {
				t.Fatalf("queue = %q, want %q", got.Queue, tt.wantQueue)
			}
			if got.RealtimeToken == "" || got.RTCToken == "" {
				t.Fatal("client tokens were not returned")
			}
			wantPaths := []string{
				"/v1/realtime/channel/create", "/v1/rtc/room/create", "/v1/realtime/publish",
				"/v1/realtime/token/issue", "/v1/rtc/token/issue",
			}
			if !reflect.DeepEqual(paths, wantPaths) {
				t.Fatalf("paths = %v, want %v", paths, wantPaths)
			}
		})
	}
}

func TestClientRetriesRateLimitAfterDecodingEnvelope(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"message":"retry later"},"metadata":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"metadata":{}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", server.Client())
	client.sleep = func(context.Context, time.Duration) error { return nil }
	err := client.post(context.Background(), "/v1/realtime/channel/create", map[string]string{
		"channel": "matter-M-204", "type": "private", "vendor": "auto",
	}, "stable-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
