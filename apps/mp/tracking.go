package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
	"github.com/jewell-lgtm/monkeypuzzle/internal/trackingclient"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newTrackingCommand()) }

func newTrackingCommand() *cobra.Command {
	parent := &cobra.Command{Use: "tracking", Short: "Opt in to publishing piece progress to mp-server",
		Long: `Explicit progress snapshots in mp-server. No other mp command contacts the
server or requires server configuration. No hooks or background synchronization.

Set MP_SERVER_URL and MP_SERVER_TOKEN, then use:
  mp tracking put --state working --note 'Implementing the API'
  mp tracking list
  mp tracking delete

identity is local-only. All commands emit JSON. See docs/server-tracking.md.`}
	var endpoint string
	parent.PersistentFlags().StringVar(&endpoint, "server", "", "mp-server base URL (default: MP_SERVER_URL)")
	for _, verb := range []string{"put", "delete", "list", "identity"} {
		var root, piece, state, note string
		cmd := &cobra.Command{Use: verb, Args: cobra.NoArgs}
		switch verb {
		case "put":
			cmd.Short = "Create or replace the current piece's progress snapshot"
		case "delete":
			cmd.Short = "Stop tracking the current piece (safe to repeat)"
		case "list":
			cmd.Short = "List your tracked pieces across all machines and projects"
		case "identity":
			cmd.Short = "Show persistent local machine/project/piece identifiers"
		}
		if verb != "list" {
			cmd.Flags().StringVar(&root, "project-root", "", "Explicit original project path; requires --piece (works after worktree removal)")
			cmd.Flags().StringVar(&piece, "piece", "", "Explicit piece name; requires --project-root")
			cmd.MarkFlagsRequiredTogether("project-root", "piece")
		}
		if verb == "put" {
			cmd.Flags().StringVar(&state, "state", "", "todo, working, blocked, review or done")
			cmd.Flags().StringVar(&note, "note", "", "Progress note (omission clears the previous note)")
			_ = cmd.MarkFlagRequired("state")
		}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			var client *trackingclient.Client
			var err error
			if verb != "identity" {
				server := endpoint
				if server == "" {
					server = os.Getenv("MP_SERVER_URL")
				}
				client, err = trackingclient.New(server, os.Getenv("MP_SERVER_TOKEN"))
				if err != nil {
					return err
				}
			}
			var result any
			if verb == "list" {
				result, err = client.List(cmd.Context())
			} else {
				projectRoot, snapshot, e := trackingContext(cmd, root, piece)
				if e != nil {
					return e
				}
				snapshot.State, snapshot.Note = state, note
				if verb == "put" {
					if e := snapshot.Validate(); e != nil {
						return e
					}
				}
				dir, e := paths.ConfigDir()
				if e != nil {
					return e
				}
				key, e := trackingclient.Identity(filepath.Join(dir, "tracking"), projectRoot, snapshot.Piece)
				if e != nil {
					return e
				}
				switch verb {
				case "identity":
					result = key
				case "put":
					result, err = client.Put(cmd.Context(), key, snapshot)
				case "delete":
					err = client.Delete(cmd.Context(), key)
					result = map[string]bool{"deleted": true}
				}
			}
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		parent.AddCommand(cmd)
	}
	return parent
}

func trackingContext(cmd *cobra.Command, root, piece string) (string, tracking.Snapshot, error) {
	var snapshot tracking.Snapshot
	if (root == "") != (piece == "") {
		return "", snapshot, fmt.Errorf("--project-root and --piece must be supplied together")
	}
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", snapshot, err
		}
		deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupCLILoading(os.Stderr))
		status, err := newPieceHandler(deps).Status(cmd.Context(), wd)
		if err != nil {
			return "", snapshot, err
		}
		if !status.InPiece {
			return "", snapshot, fmt.Errorf("run inside an mp piece, or supply --project-root and --piece")
		}
		root, piece = status.RepoRoot, status.PieceName
		snapshot.WorktreePath = status.WorktreePath
		meta, err := piececmd.ReadPieceMetadata(status.WorktreePath, deps.FS)
		if err != nil {
			return "", snapshot, err
		}
		if meta != nil {
			snapshot.Parent = meta.Parent
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", snapshot, err
	}
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = canonical
	}
	snapshot.Machine, err = os.Hostname()
	if err != nil {
		return "", snapshot, err
	}
	snapshot.Project, snapshot.Piece = filepath.Base(root), piece
	return root, snapshot, nil
}
