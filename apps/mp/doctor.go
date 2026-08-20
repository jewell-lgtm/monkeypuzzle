package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Work with projects on remote ssh hosts",
	Long: `Remote development over ssh: mp proxies whole commands to an mp binary on
the host where the repo lives. See 'mp project add HOST:PATH', the global
--host/--dir/--project flags, and docs/remote-development.md.`,
}

var remoteDoctorCmd = &cobra.Command{
	Use:   "doctor [host]",
	Short: "Check that a remote host is ready for mp over ssh",
	Long: `Probe an ssh host and report everything the remote workflow needs: key-based
(BatchMode) ssh access, an mp binary and its version vs this one, git, tmux,
and gh with working auth. Run it once after installing mp on a new host, and
first whenever a proxied command misbehaves.

The host argument is an ssh destination (alias or user@host). With no
argument, every distinct host in the project registry is checked.

Like 'mp config', this is a diagnostic that uses positional args — there is
no JSON-stdin mode; the JSON report always goes to stdout.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRemoteDoctor,
}

func init() {
	remoteCmd.AddCommand(remoteDoctorCmd)
	rootCmd.AddCommand(remoteCmd)
}

// doctorReport is the JSON result of probing one host.
type doctorReport struct {
	Host         string `json:"host"`
	Reachable    bool   `json:"reachable"`
	SSHError     string `json:"ssh_error,omitempty"`
	MPVersion    string `json:"mp_version,omitempty"` // "missing" when not installed
	LocalVersion string `json:"local_version"`
	VersionMatch bool   `json:"version_match"`
	Git          bool   `json:"git"`
	Tmux         bool   `json:"tmux"`
	Gh           bool   `json:"gh"`
	GhAuth       bool   `json:"gh_auth"`
	// Dir/Init are set when a path was probed: is it an mp project there?
	Dir  string `json:"dir,omitempty"`
	Init bool   `json:"init,omitempty"`
}

// doctorProbe is the single shell script run on the host; one key=value line
// per check keeps it one ssh round-trip. It probes the same binary the proxy
// would run (MP_REMOTE_BIN honored) under the same PATH. With a dir it also
// reports whether that path is an mp project (init=yes|no).
func doctorProbe(dir string) string {
	script := `export PATH="$HOME/.local/bin:$PATH"
echo "mp=$(` + cli.ShQuote(remoteBin()) + ` --version 2>/dev/null || echo missing)"
echo "git=$(command -v git >/dev/null && echo yes || echo no)"
echo "tmux=$(command -v tmux >/dev/null && echo yes || echo no)"
echo "gh=$(command -v gh >/dev/null && echo yes || echo no)"
echo "gh_auth=$(gh auth status >/dev/null 2>&1 && echo yes || echo no)"`
	if dir != "" {
		script += `
echo "init=$(test -f ` + cli.ShQuote(dir+"/.monkeypuzzle/monkeypuzzle.json") + ` && echo yes || echo no)"`
	}
	return script
}

func runRemoteDoctor(cmd *cobra.Command, args []string) error {
	var hosts []string
	switch {
	case len(args) == 1:
		hosts = []string{args[0]}
	default:
		reg, err := registry.Load()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, p := range reg.Projects {
			if p.Host != "" && !seen[p.Host] {
				seen[p.Host] = true
				hosts = append(hosts, p.Host)
			}
		}
		if len(hosts) == 0 {
			return fmt.Errorf("no host given and no remote projects registered (try `mp remote doctor <host>`)")
		}
	}

	reports := make([]doctorReport, 0, len(hosts))
	ok := true
	for _, h := range hosts {
		r := probeHost(h, "")
		reports = append(reports, r)
		ok = ok && r.Reachable && r.MPVersion != "missing" && r.Git
		printDoctorHuman(r)
	}
	if err := cli.PrintJSON(reports); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("doctor found problems (see above)")
	}
	return nil
}

// probeHost runs the doctor probe on host; dir, when set, is a box-side path
// whose mp-project status is reported as Init.
func probeHost(host, dir string) doctorReport {
	r := doctorReport{Host: host, LocalVersion: resolveVersion(), Dir: dir}
	// The registry file is user-writable; a poisoned host must not reach ssh.
	if err := cli.ValidSSHDest(host); err != nil {
		r.SSHError = err.Error()
		return r
	}
	// sh -c so the POSIX probe survives fish/csh login shells.
	out, err := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "--", host, "sh -c "+cli.ShQuote(doctorProbe(dir))).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			r.SSHError = strings.TrimSpace(string(exitErr.Stderr))
		}
		if r.SSHError == "" {
			r.SSHError = err.Error()
		}
		return r
	}
	r.Reachable = true
	for _, line := range strings.Split(string(out), "\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch k {
		case "mp":
			r.MPVersion = strings.TrimPrefix(v, "mp version ")
		case "git":
			r.Git = v == "yes"
		case "tmux":
			r.Tmux = v == "yes"
		case "gh":
			r.Gh = v == "yes"
		case "gh_auth":
			r.GhAuth = v == "yes"
		case "init":
			r.Init = v == "yes"
		}
	}
	r.VersionMatch = r.MPVersion == r.LocalVersion
	return r
}

func printDoctorHuman(r doctorReport) {
	tick := func(b bool) string {
		if b {
			return cli.GlyphOK
		}
		return cli.GlyphFail
	}
	fmt.Fprintf(os.Stderr, "%s:\n", r.Host)
	if !r.Reachable {
		fmt.Fprintf(os.Stderr, "  %s ssh (BatchMode): %s\n", cli.GlyphFail, r.SSHError)
		fmt.Fprintf(os.Stderr, "    fix: check `ssh %s` works with key auth, no prompt\n", r.Host)
		return
	}
	fmt.Fprintf(os.Stderr, "  %s ssh (BatchMode)\n", cli.GlyphOK)
	if r.MPVersion == "missing" {
		fmt.Fprintf(os.Stderr, "  %s mp: not found on PATH or ~/.local/bin (install it there)\n", cli.GlyphFail)
	} else if !r.VersionMatch {
		fmt.Fprintf(os.Stderr, "  %s mp %s (local is %s — remote flags/schemas win)\n", cli.GlyphWarn, r.MPVersion, r.LocalVersion)
	} else {
		fmt.Fprintf(os.Stderr, "  %s mp %s = local\n", cli.GlyphOK, r.MPVersion)
	}
	fmt.Fprintf(os.Stderr, "  %s git  %s tmux  %s gh", tick(r.Git), tick(r.Tmux), tick(r.Gh))
	if r.Gh {
		fmt.Fprintf(os.Stderr, " (auth: %s)", tick(r.GhAuth))
	}
	fmt.Fprintln(os.Stderr)
	if r.Dir != "" {
		fmt.Fprintf(os.Stderr, "  %s %s is an mp project\n", tick(r.Init), r.Dir)
	}
}
