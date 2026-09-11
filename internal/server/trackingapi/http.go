// Package trackingapi exposes authenticated, user-scoped progress snapshots.
package trackingapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func NewHandler(st store.TrackingStore, verifier auth.TokenVerifier, metadataURL string) http.Handler {
	mux := http.NewServeMux()
	serve := func(w http.ResponseWriter, r *http.Request) {
		info := auth.TokenInfoFromContext(r.Context())
		if info == nil {
			fail(w, 401, "authentication required")
			return
		}
		uid, err := strconv.ParseInt(info.UserID, 10, 64)
		if err != nil || uid <= 0 {
			fail(w, 401, "invalid user")
			return
		}
		if r.URL.RawQuery != "" {
			fail(w, 400, "query parameters are not supported")
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			items, err := st.ListTrackedItems(r.Context(), uid)
			if err != nil {
				fail(w, 503, "storage unavailable")
				return
			}
			write(w, 200, tracking.List{Items: items})
			return
		}
		key := tracking.Key{MachineID: r.PathValue("machine"), ProjectID: r.PathValue("project"), PieceID: r.PathValue("piece")}
		if err := key.Validate(); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if r.Method == http.MethodDelete {
			if err := st.DeleteTrackedItem(r.Context(), uid, key); err != nil {
				fail(w, 503, "storage unavailable")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			fail(w, 415, "application/json required")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, tracking.MaxBody)
		defer func() { _ = r.Body.Close() }()
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var snapshot tracking.Snapshot
		if err := dec.Decode(&snapshot); err != nil {
			decodeError(w, err)
			return
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			decodeError(w, err)
			return
		}
		if err := snapshot.Validate(); err != nil {
			fail(w, 400, err.Error())
			return
		}
		item, err := st.PutTrackedItem(r.Context(), uid, key, snapshot)
		if err != nil {
			fail(w, 503, "storage unavailable")
			return
		}
		write(w, 200, item)
	}
	mux.HandleFunc("GET "+tracking.BasePath, serve)
	mux.HandleFunc("PUT "+tracking.BasePath+"/{machine}/{project}/{piece}", serve)
	mux.HandleFunc("DELETE "+tracking.BasePath+"/{machine}/{project}/{piece}", serve)
	return auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL})(mux)
}

func decodeError(w http.ResponseWriter, err error) {
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		fail(w, 413, "body exceeds 64 KiB")
		return
	}
	fail(w, 400, "expected one JSON snapshot with supported fields")
}

func fail(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]string{"error": message})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
