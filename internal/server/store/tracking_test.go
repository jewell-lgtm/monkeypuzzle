package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

// Run against a disposable database: MP_TEST_DATABASE_URL=... go test ./internal/server/store -run TestTrackingPostgres
func TestTrackingPostgresPersistence(t *testing.T) {
	dsn := os.Getenv("MP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := NewPgxStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for range 2 {
		if err := st.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Now().UnixNano()
	uid, err := st.UpsertUser(ctx, User{ExternalUserID: fmt.Sprint(stamp), Provider: "github", ForgeUserID: stamp, AccessTokenEnc: []byte{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = st.pool.Exec(ctx, "DELETE FROM users WHERE id=$1", uid) }()
	key := tracking.Key{MachineID: "m", ProjectID: "p", PieceID: "piece"}
	snapshot := tracking.Snapshot{Machine: "mac", Project: "mp", Piece: "feature", State: "working"}
	first, err := st.PutTrackedItem(ctx, uid, key, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.PutTrackedItem(ctx, uid, key, snapshot)
	if err != nil || second != first {
		t.Fatalf("retry changed timestamps: %+v %v", second, err)
	}
	reopened, err := NewPgxStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	items, err := reopened.ListTrackedItems(ctx, uid)
	if err != nil || len(items) != 1 || items[0] != first {
		t.Fatalf("persistence: %+v %v", items, err)
	}
	items, err = reopened.ListTrackedItems(ctx, uid+1)
	if err != nil || len(items) != 0 {
		t.Fatalf("cross-user list: %+v %v", items, err)
	}
	if err := reopened.DeleteTrackedItem(ctx, uid+1, key); err != nil {
		t.Fatal(err)
	}
	snapshot.State = "done"
	updated, err := reopened.PutTrackedItem(ctx, uid, key, snapshot)
	if err != nil || updated.CreatedAt != first.CreatedAt || !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("update: %+v %v", updated, err)
	}
	for range 2 {
		if err := reopened.DeleteTrackedItem(ctx, uid, key); err != nil {
			t.Fatal(err)
		}
	}
	items, err = st.ListTrackedItems(ctx, uid)
	if err != nil || len(items) != 0 {
		t.Fatalf("delete: %+v %v", items, err)
	}
}
