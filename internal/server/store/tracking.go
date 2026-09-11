package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

// TrackingStore owns explicitly published snapshots, separately from forge sync.
// Every operation is scoped to the authenticated local user, including DELETE.
type TrackingStore interface {
	PutTrackedItem(context.Context, int64, tracking.Key, tracking.Snapshot) (tracking.Item, error)
	DeleteTrackedItem(context.Context, int64, tracking.Key) error
	ListTrackedItems(context.Context, int64) ([]tracking.Item, error)
}

func (s *PgxStore) PutTrackedItem(ctx context.Context, uid int64, key tracking.Key, snapshot tracking.Snapshot) (tracking.Item, error) {
	item := tracking.Item{Key: key, Snapshot: snapshot}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return item, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO tracked_items (user_id, machine_id, project_id, piece_id, payload)
		VALUES ($1,$2,$3,$4,$5::jsonb)
		ON CONFLICT (user_id,machine_id,project_id,piece_id) DO UPDATE SET
		payload=EXCLUDED.payload,
		updated_at=CASE WHEN tracked_items.payload IS DISTINCT FROM EXCLUDED.payload
			THEN clock_timestamp() ELSE tracked_items.updated_at END
		RETURNING created_at, updated_at`, uid, key.MachineID, key.ProjectID, key.PieceID, data).Scan(&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *PgxStore) DeleteTrackedItem(ctx context.Context, uid int64, key tracking.Key) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tracked_items WHERE user_id=$1 AND machine_id=$2 AND project_id=$3 AND piece_id=$4`, uid, key.MachineID, key.ProjectID, key.PieceID)
	return err
}

func (s *PgxStore) ListTrackedItems(ctx context.Context, uid int64) ([]tracking.Item, error) {
	rows, err := s.pool.Query(ctx, `SELECT machine_id, project_id, piece_id, payload, created_at, updated_at
		FROM tracked_items WHERE user_id=$1 ORDER BY updated_at DESC, machine_id, project_id, piece_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []tracking.Item{}
	for rows.Next() {
		var item tracking.Item
		var data []byte
		if err := rows.Scan(&item.MachineID, &item.ProjectID, &item.PieceID, &data, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &item.Snapshot); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type trackedKey struct {
	UserID int64
	Key    tracking.Key
}

func (m *MemoryStore) PutTrackedItem(_ context.Context, uid int64, key tracking.Key, snapshot tracking.Snapshot) (tracking.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[uid]; !ok {
		return tracking.Item{}, ErrNotFound
	}
	if m.tracked == nil {
		m.tracked = map[trackedKey]tracking.Item{}
	}
	k := trackedKey{uid, key}
	item, exists := m.tracked[k]
	if !exists {
		item = tracking.Item{Key: key, CreatedAt: time.Now().UTC()}
	}
	if !exists || item.Snapshot != snapshot {
		item.Snapshot = snapshot
		item.UpdatedAt = time.Now().UTC()
	}
	m.tracked[k] = item
	return item, nil
}

func (m *MemoryStore) DeleteTrackedItem(_ context.Context, uid int64, key tracking.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tracked, trackedKey{uid, key})
	return nil
}

func (m *MemoryStore) ListTrackedItems(_ context.Context, uid int64) ([]tracking.Item, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := []tracking.Item{}
	for key, item := range m.tracked {
		if key.UserID == uid {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].Path() < items[j].Path()
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}
