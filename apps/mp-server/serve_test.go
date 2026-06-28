package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePinger is a dbPinger whose Ping result is fixed by err.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// TestHealthz: liveness returns 200 "ok" unconditionally.
func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	healthzHandler(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Result().Body); string(body) != "ok" {
		t.Fatalf("healthz body = %q, want %q", body, "ok")
	}
}

// TestReadyz_DBHealthy: readiness returns 200 when the DB ping succeeds; a
// failing best-effort Temporal check must NOT flip it to non-200.
func TestReadyz_DBHealthy(t *testing.T) {
	h := newReadyzHandler(fakePinger{}, func(context.Context) error { return errors.New("temporal flaky") })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200 (temporal failure should be best-effort)", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Result().Body); string(body) != "ok" {
		t.Fatalf("readyz body = %q, want %q", body, "ok")
	}
}

// TestReadyz_DBDown: readiness returns 503 with a generic body when the DB ping
// fails. The raw driver error (which can carry DB user/database/host:port) must
// be logged server-side only, never disclosed to unauthenticated probe callers.
func TestReadyz_DBDown(t *testing.T) {
	rawErr := "failed to connect to `user=mp database=mp host=db.internal:5432`: connection refused"
	h := newReadyzHandler(fakePinger{err: errors.New(rawErr)}, nil)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", rec.Code)
	}
	got, _ := io.ReadAll(rec.Result().Body)
	body := string(got)
	if !strings.Contains(body, "database unreachable") {
		t.Fatalf("readyz body = %q, want it to contain %q", body, "database unreachable")
	}
	for _, leak := range []string{"user=", "db unreachable:", rawErr} {
		if strings.Contains(body, leak) {
			t.Fatalf("readyz body = %q leaks %q to clients", body, leak)
		}
	}
}
