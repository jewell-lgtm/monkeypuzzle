package store

import (
	"context"
	"fmt"
)

func (s *PgxStore) EnsureRegistryUser(ctx context.Context, externalID, displayName, avatarURL string) (int64, error) {
	if externalID == "" {
		return 0, fmt.Errorf("store: missing authenticated subject")
	}
	// Reserve a unique negative forge placeholder without changing existing schema.
	// Conflict resolution uses the authenticated subject, never a display name.
	const query = `INSERT INTO users (external_user_id, provider, github_user_id, github_login, avatar_url, access_token_enc)
 VALUES ($1, 'workos', -nextval('users_id_seq'), $2, $3, ''::bytea)
 ON CONFLICT (external_user_id) DO UPDATE SET updated_at = now()
 RETURNING id`
	var id int64
	err := s.pool.QueryRow(ctx, query, externalID, displayName, avatarURL).Scan(&id)
	return id, err
}

func (m *MemoryStore) EnsureRegistryUser(_ context.Context, externalID, displayName, avatarURL string) (int64, error) {
	if externalID == "" {
		return 0, fmt.Errorf("store: missing authenticated subject")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.usersByExt[externalID]; ok {
		return id, nil
	}
	m.nextUser++
	u := User{ID: m.nextUser, ExternalUserID: externalID, Provider: "workos", ForgeUserID: -m.nextUser, ForgeLogin: displayName, AvatarURL: avatarURL, AccessTokenEnc: []byte{}}
	m.users[u.ID] = u
	m.usersByExt[externalID] = u.ID
	m.usersByForge[forgeKey(u.Provider, u.ForgeUserID)] = u.ID
	return u.ID, nil
}
