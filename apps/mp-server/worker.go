package main

import (
	"context"
	"fmt"
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	forgegithub "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/github"
	forgegitlab "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/gitlab"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	syncpkg "github.com/jewell-lgtm/monkeypuzzle/internal/server/sync"
)

// runWorker runs the Temporal worker that executes the GitHub -> Postgres sync.
func runWorker() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()

	st, err := store.NewPgxStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	cipher, err := crypto.NewAESGCMCipher(cfg.TokenEncryptionKey)
	if err != nil {
		return err
	}

	tc, err := client.Dial(client.Options{HostPort: cfg.TemporalHostPort})
	if err != nil {
		return fmt.Errorf("temporal dial: %w", err)
	}
	defer tc.Close()

	w := worker.New(tc, syncpkg.TaskQueue, worker.Options{})
	syncpkg.RegisterWorker(w, &syncpkg.Activities{
		Store: st,
		Forge: forge.Registry{
			"github": forgegithub.NewFactory(),
			"gitlab": forgegitlab.NewFactory(cfg.GitLabBaseURL),
		},
		Cipher: cipher,
	})

	log.Printf("mp-worker polling task queue %q", syncpkg.TaskQueue)
	return w.Run(worker.InterruptCh())
}
