package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// client calls a workspace's API with an API token (Sustain's docs/API.md): JSON both ways, a POST with an idempotency
// key so it's made once however often it's sent, and what Sustain asks to be sent again, sent again: too many at once
// (429), unavailable just now (503), or a key's first request still being answered (409).
type client struct {
	base      *url.URL
	token     string
	http      *http.Client
	userAgent string
	// tries is how many times a request is sent at most, and wait how long it waits between them unless Sustain says.
	tries int
	wait  func(attempt int, said time.Duration) time.Duration
}

// apiError is a refusal, as Sustain's envelope says it, and how long Sustain asked to be left before it's sent again.
type apiError struct {
	Status     int
	Code       string
	Message    string
	RequestID  string
	retryAfter time.Duration
}

func (e *apiError) Error() string {
	text := fmt.Sprintf("%s (%s, %d)", e.Message, e.Code, e.Status)
	if e.RequestID != "" {
		text += ", request " + e.RequestID
	}
	return text
}

// refusedAs says whether err is Sustain's refusal with one of codes.
func refusedAs(err error, codes ...string) bool {
	var refusal *apiError
	return errors.As(err, &refusal) && has(codes, refusal.Code)
}

// notFound says whether err is Sustain saying there's no such thing, under whichever code it names it.
func notFound(err error) bool {
	var refusal *apiError
	return errors.As(err, &refusal) && refusal.Status == http.StatusNotFound
}

func newClient(address, token, version string) (*client, error) {
	base, err := url.Parse(strings.TrimRight(address, "/"))
	if err != nil || base.Host == "" || base.Path != "" || base.RawQuery != "" || base.User != nil {
		return nil, fmt.Errorf("%q isn't a workspace's address, as https://acme.sustain.watch", address)
	}
	host := base.Hostname()
	local := host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1"
	if base.Scheme != "https" && !(base.Scheme == "http" && local) {
		return nil, fmt.Errorf("%q isn't https, as a workspace's address is", address)
	}
	return &client{
		base:      base,
		token:     token,
		http:      &http.Client{Timeout: 60 * time.Second},
		userAgent: "terraform-provider-sustain/" + version,
		tries:     6,
		wait:      backoff,
	}, nil
}

// backoff waits as Sustain said, or a second, then two, four and so on, up to a minute.
func backoff(attempt int, said time.Duration) time.Duration {
	if said > 0 {
		return min(said, time.Minute)
	}
	return min(time.Second<<attempt, time.Minute)
}

// call sends one request to the API and reads its answer's JSON, numbers kept as written.
func (c *client) call(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	key := ""
	if method == http.MethodPost {
		key = idempotencyKey()
	}
	var last error
	for attempt := 0; attempt < c.tries; attempt++ {
		if attempt > 0 {
			var said time.Duration
			var refusal *apiError
			if errors.As(last, &refusal) {
				said = refusal.retryAfter
			}
			select {
			case <-ctx.Done():
				return nil, errors.Join(ctx.Err(), last)
			case <-time.After(c.wait(attempt-1, said)):
			}
		}
		answer, again, err := c.send(ctx, method, path, payload, key)
		if err == nil || !again {
			return answer, err
		}
		last = err
	}
	return nil, last
}

// send sends a request once, and says whether it may be sent again.
func (c *client) send(ctx context.Context, method, path string, payload []byte, key string) (map[string]any, bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+path, bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	} else {
		req.Body = nil
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, err
		}
		// Never sent, as no connection was made: sent again, whatever it is. Lost on the way: sent again if sending it
		// twice does nothing twice, as a read, a deletion and a POST with its key don't; a change might have been made.
		var dial *net.OpError
		unsent := errors.As(err, &dial) && dial.Op == "dial"
		return nil, unsent || method != http.MethodPatch, err
	}
	defer res.Body.Close()
	text, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, method != http.MethodPatch, err
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		answer := map[string]any{}
		if len(bytes.TrimSpace(text)) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(text))
			decoder.UseNumber()
			if err := decoder.Decode(&answer); err != nil {
				return nil, false, fmt.Errorf("sustain's answer to %s %s isn't JSON: %w", method, path, err)
			}
		}
		return answer, false, nil
	}
	refusal := refusalOf(res, text)
	again := res.StatusCode == http.StatusTooManyRequests ||
		res.StatusCode == http.StatusServiceUnavailable ||
		(res.StatusCode == http.StatusConflict && refusal.Code == "IDEMPOTENCY_KEY_IN_USE")
	return nil, again, refusal
}

// refusalOf reads Sustain's envelope, `{"error": {"code", "message", "request_id"}}`, from a refusal.
func refusalOf(res *http.Response, text []byte) *apiError {
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	refusal := &apiError{Status: res.StatusCode, RequestID: res.Header.Get("x-request-id")}
	if json.Unmarshal(text, &envelope) == nil && envelope.Error.Code != "" {
		refusal.Code, refusal.Message = envelope.Error.Code, envelope.Error.Message
		if envelope.Error.RequestID != "" {
			refusal.RequestID = envelope.Error.RequestID
		}
	} else {
		refusal.Code, refusal.Message = strconv.Itoa(res.StatusCode), http.StatusText(res.StatusCode)
	}
	if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
		refusal.retryAfter = time.Duration(seconds) * time.Second
	}
	return refusal
}

// idempotencyKey is a new request's key: random, and the same each time that request is sent.
func idempotencyKey() string {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return fmt.Sprintf("terraform-%x", bytes)
}
