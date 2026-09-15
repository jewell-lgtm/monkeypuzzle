package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRegistryAccounts(t *testing.T) {
	run := func(t *testing.T, s Store) {
		ctx := context.Background()
		prefix := fmt.Sprint(time.Now().UnixNano())
		first, err := s.EnsureRegistryUser(ctx, prefix+"first", "Same name", "")
		if err != nil {
			t.Fatal(err)
		}
		second, err := s.EnsureRegistryUser(ctx, prefix+"second", "Same name", "")
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := s.EnsureRegistryUser(ctx, prefix+"first", "Changed name", "")
		if err != nil {
			t.Fatal(err)
		}
		if first == second || first != repeated {
			t.Fatal("subject isolation/idempotency failed")
		}
		existing, err := s.UpsertUser(ctx, User{ExternalUserID: prefix + "existing", Provider: "github", ForgeUserID: time.Now().UnixNano(), AccessTokenEnc: []byte("encrypted-token")})
		if err != nil {
			t.Fatal(err)
		}
		preserved, err := s.EnsureRegistryUser(ctx, prefix+"existing", "Name", "")
		if err != nil || preserved != existing {
			t.Fatalf("existing account: %v", err)
		}
		user, err := s.GetUserByID(ctx, preserved)
		if err != nil || string(user.AccessTokenEnc) != "encrypted-token" || user.Provider != "github" {
			t.Fatal("existing credentials changed")
		}
		if _, err := s.EnsureRegistryUser(ctx, "", "", ""); err == nil {
			t.Fatal("empty subject accepted")
		}
	}
	t.Run("memory", func(t *testing.T) { run(t, NewMemoryStore()) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("MP_TEST_DATABASE_URL")
		if dsn == "" {
			t.Skip("MP_TEST_DATABASE_URL not set")
		}
		s, err := NewPgxStore(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err := s.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		run(t, s)
	})
}
