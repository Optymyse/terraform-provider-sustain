package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// served is a stand-in for Sustain's API, answering each request in turn as answers says, and noting what it was sent.
type served struct {
	mu       sync.Mutex
	answers  []func(http.ResponseWriter, *http.Request)
	requests []*http.Request
	bodies   []string
}

func (s *served) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	body, _ := io.ReadAll(r.Body)
	s.requests = append(s.requests, r)
	s.bodies = append(s.bodies, string(body))
	answer := s.answers[min(len(s.requests), len(s.answers))-1]
	s.mu.Unlock()
	answer(w, r)
}

func answering(status int, body string, headers ...string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		for index := 0; index+1 < len(headers); index += 2 {
			w.Header().Set(headers[index], headers[index+1])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// dropped closes the connection without an answer, as a request lost on the way is.
func dropped(w http.ResponseWriter, _ *http.Request) {
	conn, _, _ := w.(http.Hijacker).Hijack()
	_ = conn.Close()
}

// clientOf is a client of a stand-in, its waits noted rather than waited.
func clientOf(t *testing.T, s *served) (*client, *[]time.Duration) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	c, err := newClient(strings.Replace(server.URL, "127.0.0.1", "localhost", 1), "sw_admin_eu_test", "test")
	if err != nil {
		t.Fatal(err)
	}
	waits := []time.Duration{}
	c.wait = func(attempt int, said time.Duration) time.Duration {
		waits = append(waits, backoff(attempt, said))
		return 0
	}
	return c, &waits
}

func TestAPostIsSentAgainWithItsKeyWhenSustainSaysToWait(t *testing.T) {
	s := &served{answers: []func(http.ResponseWriter, *http.Request){
		answering(429, `{"error":{"code":"RATE_LIMITED","message":"Too many."}}`, "Retry-After", "60"),
		answering(503, `{"error":{"code":"WORKSPACE_UNAVAILABLE","message":"Not now."}}`),
		answering(201, `{"service":{"id":"svc_1","sla":99.95}}`),
	}}
	c, waits := clientOf(t, s)
	answer, err := c.call(context.Background(), http.MethodPost, "/api/v1/services", map[string]any{"name": "Shop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.requests) != 3 {
		t.Fatalf("sent %d times", len(s.requests))
	}
	key := s.requests[0].Header.Get("Idempotency-Key")
	for _, each := range s.requests {
		if each.Header.Get("Idempotency-Key") != key || key == "" {
			t.Errorf("sent with key %q, then %q", key, each.Header.Get("Idempotency-Key"))
		}
		if each.Header.Get("Authorization") != "Bearer sw_admin_eu_test" || each.Header.Get("Content-Type") != "application/json" {
			t.Errorf("sent with %v", each.Header)
		}
	}
	if (*waits)[0] != time.Minute || (*waits)[1] != 2*time.Second {
		t.Errorf("waited %v", *waits)
	}
	// Numbers are kept as they're written.
	if answer["service"].(map[string]any)["sla"] != json.Number("99.95") {
		t.Errorf("read %v", answer)
	}
}

func TestARefusalIsSustainsOwnWords(t *testing.T) {
	s := &served{answers: []func(http.ResponseWriter, *http.Request){
		answering(404, `{"error":{"code":"SERVICE_NOT_FOUND","message":"No such service.","request_id":"8f2a1c3b4d5e6f70"}}`),
	}}
	c, _ := clientOf(t, s)
	_, err := c.call(context.Background(), http.MethodGet, "/api/v1/services/svc_9", nil)
	if !notFound(err) || err.Error() != "No such service. (SERVICE_NOT_FOUND, 404), request 8f2a1c3b4d5e6f70" {
		t.Errorf("refused as %v", err)
	}
	if len(s.requests) != 1 {
		t.Errorf("sent %d times", len(s.requests))
	}
}

func TestOnlyWhatDoesNothingTwiceIsSentAgainWhenItsLost(t *testing.T) {
	s := &served{answers: []func(http.ResponseWriter, *http.Request){dropped, answering(200, `{"monitor":{}}`)}}
	c, _ := clientOf(t, s)
	if _, err := c.call(context.Background(), http.MethodGet, "/api/v1/monitors/m1", nil); err != nil {
		t.Errorf("a read lost on the way wasn't sent again: %v", err)
	}
	s = &served{answers: []func(http.ResponseWriter, *http.Request){dropped, answering(200, `{"monitor":{}}`)}}
	c, _ = clientOf(t, s)
	if _, err := c.call(context.Background(), http.MethodPatch, "/api/v1/monitors/m1", map[string]any{"name": "x"}); err == nil {
		t.Error("a change lost on the way was sent again")
	}
	if len(s.requests) != 1 {
		t.Errorf("sent %d times", len(s.requests))
	}
}

func TestAWorkspacesAddressIsHttpsButOnThisMachine(t *testing.T) {
	for address, ok := range map[string]bool{
		"https://acme.sustain.watch":   true,
		"https://acme.sustain.watch/":  true,
		"http://acme.localhost:4188":   true,
		"http://acme.sustain.watch":    false,
		"https://acme.sustain.watch/x": false,
		"acme.sustain.watch":           false,
	} {
		if _, err := newClient(address, "token", "test"); (err == nil) != ok {
			t.Errorf("%s: %v", address, err)
		}
	}
}
