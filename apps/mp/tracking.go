package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
	"github.com/jewell-lgtm/monkeypuzzle/internal/trackingclient"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newTrackingCommand())
	rootCmd.AddCommand(newSettleCommand())
}

func newTrackingCommand() *cobra.Command {
	parent := &cobra.Command{Use: "tracking", Short: "Opt in to publishing piece progress to mp-server",
		Long: `Explicit progress snapshots in mp-server. No other mp command contacts the
server or requires server configuration. No hooks or background synchronization.

Set MP_SERVER_URL and MP_SERVER_TOKEN, then use:
  mp tracking report --state working --note 'Implementing the API'
  mp tracking list
  mp settle

identity is local-only. All commands emit JSON. See docs/server-tracking.md.`}
	var endpoint string
	parent.PersistentFlags().StringVar(&endpoint, "server", "", "mp-server base URL (default: MP_SERVER_URL)")
	for _, verb := range []string{"report", "delete", "list", "identity"} {
		var root, piece, state, note string
		cmd := &cobra.Command{Use: verb, Args: cobra.NoArgs}
		switch verb {
		case "report":
			cmd.Aliases = []string{"put"}
			cmd.Short = "Report the current piece's progress"
		case "delete":
			cmd.Hidden = true // compatibility spelling; the piece-shaped verb is `mp settle`
			cmd.Short = "Remove the current piece from the registry (use mp settle)"
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
		if verb == "report" {
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
				if verb == "report" {
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
				case "report":
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

func newSettleCommand() *cobra.Command {
	var endpoint, projectRoot, piece string
	var jsonOutput, schema bool
	cmd := &cobra.Command{
		Use:   "settle [piece]",
		Short: "Remove a published piece from your private registry",
		Long: `Stop showing a piece in mp-server. Settling is deliberately not a workflow
outcome: it does not touch the branch, worktree, PR, session, or local piece.
Use mp done after a merge, or mp abandon when you intentionally discard work.

Defaults to the piece you're standing in. From the main repository, name a
piece positionally or with --piece. Use global --project before the verb to
select another registered project. Safe to repeat.

  mp settle old-feature
  mp --project api settle old-feature
  echo '{"piece":"old-feature"}' | mp settle
  mp settle --schema`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completePieceNames,
	}
	cmd.Flags().StringVar(&piece, "piece", "", "Published piece to settle (default: the piece you're in)")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Original project path (advanced; requires a piece)")
	cmd.Flags().StringVar(&endpoint, "server", "", "mp-server base URL (default: MP_SERVER_URL)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON even on a terminal")
	cmd.Flags().BoolVar(&schema, "schema", false, "Print an example input document and exit")
	_ = cmd.RegisterFlagCompletionFunc("piece", completePieceNames)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if schema {
			return printIndentedJSON(cmd, settleInput{Piece: "old-feature", ProjectRoot: "/code/project"})
		}
		in := settleInput{ProjectRoot: projectRoot}
		if cli.HasStdinData() {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			if err := json.Unmarshal(data, &in); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
		}
		if cmd.Flags().Changed("project-root") {
			in.ProjectRoot = projectRoot
		}
		selector, err := pieceSelector(args, selectorFlag{"--piece", piece})
		if err != nil {
			return err
		}
		if selector != "" {
			in.Piece = selector
		}
		root, snapshot, err := settleContext(cmd, in.ProjectRoot, in.Piece)
		if err != nil {
			return err
		}
		server := endpoint
		if server == "" {
			server = os.Getenv("MP_SERVER_URL")
		}
		client, err := trackingclient.New(server, os.Getenv("MP_SERVER_TOKEN"))
		if err != nil {
			return err
		}
		dir, err := paths.ConfigDir()
		if err != nil {
			return err
		}
		key, err := trackingclient.LookupIdentity(filepath.Join(dir, "tracking"), root, snapshot.Piece)
		if err != nil {
			return err
		}
		if err := client.Delete(cmd.Context(), key); err != nil {
			return err
		}
		result := map[string]any{"settled": true, "project": snapshot.Project, "piece": snapshot.Piece}
		if jsonOutput || !cli.IsTerminal() || !cli.IsStdoutTerminal() {
			return printIndentedJSON(cmd, result)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "%s Settled piece: %s/%s\n", cli.GlyphOK, snapshot.Project, snapshot.Piece)
		return nil
	}
	return cmd
}

type settleInput struct {
	Piece       string `json:"piece,omitempty"`
	ProjectRoot string `json:"project_root,omitempty"`
}

func printIndentedJSON(cmd *cobra.Command, value any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

// settleContext gives the top-level verb the same selector shape as status,
// done, and abandon while retaining the stable project-path identity used by
// the registry protocol.
func settleContext(cmd *cobra.Command, projectRoot, selector string) (string, tracking.Snapshot, error) {
	if projectRoot != "" && selector == "" {
		return "", tracking.Snapshot{}, fmt.Errorf("--project-root requires a piece")
	}
	if selector == "" {
		return trackingContext(cmd, "", "")
	}
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", tracking.Snapshot{}, err
		}
		deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupCLILoading(os.Stderr))
		status, err := newPieceHandler(deps).Status(cmd.Context(), wd)
		if err != nil {
			return "", tracking.Snapshot{}, err
		}
		if status.RepoRoot == "" {
			return "", tracking.Snapshot{}, fmt.Errorf("run inside an mp project, use global --project before settle, or supply --project-root")
		}
		projectRoot = status.RepoRoot
	}
	return trackingContext(cmd, projectRoot, selector)
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
