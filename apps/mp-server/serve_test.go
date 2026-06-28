package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

// TestReadyz_DBDown: readiness returns 503 with a reason when the DB ping fails.
func TestReadyz_DBDown(t *testing.T) {
	h := newReadyzHandler(fakePinger{err: errors.New("connection refused")}, nil)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if len(body) == 0 {
		t.Fatal("readyz 503 should include a short reason")
	}
}
