package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// EnvShellInit is exported by the `mp shell-init` wrapper, so doctor can tell
// whether mp is able to move the caller's shell.
const EnvShellInit = "MP_SHELL_INIT"

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check this machine's mp setup",
	Long: `Report how mp is set up here and what that means for your workflow: which
multiplexer is configured and whether you are inside it, whether the shell
wrapper is loaded, what ` + "`mp open`" + ` will run, and whether this repo is a project
with agent reporting wired up.

It changes nothing. For a remote box, see ` + "`mp remote doctor`" + `.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

var flagDoctorJSON bool

func init() {
	doctorCmd.Flags().BoolVar(&flagDoctorJSON, "json", false, "Output JSON even on a terminal")
	rootCmd.AddCommand(doctorCmd)
}

// doctorCheck is one line of the report: what was checked, what was found, and
// what to do when that is not enough.
type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | info
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

type localDoctorReport struct {
	Version string        `json:"version"`
	Checks  []doctorCheck `json:"checks"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	report := localDoctorReport{Version: resolveVersion()}
	add := func(name, status, detail, fix string) {
		report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Detail: detail, Fix: fix})
	}

	// Config file
	cfgPath, _ := config.UserConfigPath()
	if config.UserConfigExists() {
		add("config", "ok", cfgPath, "")
	} else {
		add("config", "info", "not written yet (defaults in use)", "any mp command offers the first-run wizard on a terminal")
	}

	cfg, cfgErr := config.LoadUserConfig()
	if cfgErr != nil {
		add("config", "warn", fmt.Sprintf("unreadable: %v", cfgErr), "fix or delete "+cfgPath)
		cfg = config.DefaultUserConfig()
	}

	// Multiplexer: configured, installed, and are we inside it?
	osExec := adapters.NewOSExec()
	switch mux, err := adapters.NewMultiplexer(cfg.Multiplexer, osExec); {
	case err != nil:
		add("multiplexer", "warn", fmt.Sprintf("%v", err), "mp config set multiplexer none")
	case adapters.IsNoopMultiplexer(mux):
		add("multiplexer", "ok", "none — mp prints worktree paths instead of managing sessions", "")
	case !mux.IsInstalled(ctx):
		add("multiplexer", "warn", cfg.Multiplexer+" is configured but not installed", "install it, or `mp config set multiplexer none`")
	case !mux.InSession():
		add("multiplexer", "warn", cfg.Multiplexer+" is configured but this terminal is not inside it — mp will print paths", "start "+cfg.Multiplexer+", or `mp config set multiplexer none`")
	default:
		add("multiplexer", "ok", cfg.Multiplexer+" (this terminal is inside it)", "")
	}

	// Shell wrapper
	if os.Getenv(EnvShellInit) == "1" {
		add("shell-init", "ok", "loaded — mp follows you into the worktree", "")
	} else {
		add("shell-init", "info", "not loaded — mp prints the path, your shell stays put", `eval "$(mp shell-init `+currentShell()+`)"`)
	}

	// Opener
	if opener := openerTemplate(""); opener != "" {
		add("open_command", "ok", opener, "")
	} else {
		add("open_command", "info", "unset — `mp open` only prints the path", "mp config set open_command 'code {path}'")
	}

	// git, and the forge CLI this project needs
	if _, err := osExec.Run(ctx, "git", "--version"); err != nil {
		add("git", "warn", "not found on PATH", "install git")
	} else {
		add("git", "ok", "present", "")
	}

	// Project-local checks
	root, state := classifyCwd(ctx)
	switch state {
	case cwdInProject:
		add("project", "ok", root, "")
		addForgeCheck(ctx, &report, root)
		addClaudeHookCheck(&report, root)
	case cwdRepoNotProject:
		add("project", "info", root+" is a git repo but not an mp project", "mp init")
	default:
		add("project", "info", "not inside a git repository", "cd into a repo, or use `mp go`")
	}

	printDoctorReport(report)
	return emitResult(report, flagDoctorJSON)
}

// addForgeCheck reports the forge CLI this project's provider needs.
func addForgeCheck(ctx context.Context, report *localDoctorReport, root string) {
	provider := "github"
	if data, err := os.ReadFile(projectdir.ConfigFilePath(root)); err == nil {
		var cfg struct {
			Project struct {
				PRProvider string `json:"pr_provider"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.Project.PRProvider != "" {
			provider = cfg.Project.PRProvider
		}
	}
	bin, authArgs := "gh", []string{"auth", "status"}
	if provider == "gitlab" {
		bin, authArgs = "glab", []string{"auth", "status"}
	}
	add := func(status, detail, fix string) {
		report.Checks = append(report.Checks, doctorCheck{Name: bin, Status: status, Detail: detail, Fix: fix})
	}
	if _, err := exec.LookPath(bin); err != nil {
		add("warn", "not found on PATH ("+provider+" project)", "install "+bin+" to use `mp pr create`")
		return
	}
	if err := runQuiet(ctx, bin, authArgs...); err != nil {
		add("warn", "installed but not authenticated", bin+" auth login")
		return
	}
	add("ok", "installed and authenticated", "")
}

// addClaudeHookCheck reports whether agent status reporting is wired up here.
// Without a multiplexer it is the only way mp sees agents at all.
func addClaudeHookCheck(report *localDoctorReport, root string) {
	add := func(status, detail, fix string) {
		report.Checks = append(report.Checks, doctorCheck{Name: "agent reporting", Status: status, Detail: detail, Fix: fix})
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		add("info", "claude hooks not installed in this repo", "mp integration install claude")
		return
	}
	if !strings.Contains(string(data), "mp agent report") {
		add("info", "claude settings exist but don't report to mp", "mp integration install claude")
		return
	}
	add("ok", "claude hooks report to mp", "")
}

// printDoctorReport writes the human report to stderr, keeping stdout for JSON.
func printDoctorReport(r localDoctorReport) {
	fmt.Fprintf(os.Stderr, "mp %s\n", r.Version)
	for _, c := range r.Checks {
		glyph := cli.GlyphOK
		switch c.Status {
		case "warn":
			glyph = cli.GlyphWarn
		case "info":
			glyph = "·"
		}
		fmt.Fprintf(os.Stderr, "  %s %-16s %s\n", glyph, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(os.Stderr, "      %s\n", c.Fix)
		}
	}
}

// currentShell is the basename of $SHELL, defaulting to zsh — only used to
// print a copy-pasteable shell-init line.
func currentShell() string {
	sh := filepath.Base(strings.TrimSpace(os.Getenv("SHELL")))
	switch sh {
	case "bash", "zsh", "fish":
		return sh
	default:
		return "zsh"
	}
}

// runQuiet runs a command, discarding its output, reporting only success.
func runQuiet(ctx context.Context, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout, c.Stderr = nil, nil
	return c.Run()
}
