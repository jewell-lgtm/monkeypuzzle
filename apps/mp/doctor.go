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
argument, every distinct host in the project registry is checked.`,
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
}

// doctorProbe is the single shell script run on the host; one key=value line
// per check keeps it one ssh round-trip.
const doctorProbe = `export PATH="$HOME/.local/bin:$PATH"
echo "mp=$(mp --version 2>/dev/null || echo missing)"
echo "git=$(command -v git >/dev/null && echo yes || echo no)"
echo "tmux=$(command -v tmux >/dev/null && echo yes || echo no)"
echo "gh=$(command -v gh >/dev/null && echo yes || echo no)"
echo "gh_auth=$(gh auth status >/dev/null 2>&1 && echo yes || echo no)"`

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
		r := probeHost(h)
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

func probeHost(host string) doctorReport {
	r := doctorReport{Host: host, LocalVersion: resolveVersion()}
	out, err := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, "--", doctorProbe).Output()
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
		}
	}
	r.VersionMatch = r.MPVersion == r.LocalVersion
	return r
}

func printDoctorHuman(r doctorReport) {
	w := os.Stderr
	tick := func(b bool) string {
		if b {
			return "✓"
		}
		return "✗"
	}
	fmt.Fprintf(w, "%s:\n", r.Host)
	if !r.Reachable {
		fmt.Fprintf(w, "  ✗ ssh (BatchMode): %s\n", r.SSHError)
		fmt.Fprintf(w, "    fix: check `ssh %s` works with key auth, no prompt\n", r.Host)
		return
	}
	fmt.Fprintf(w, "  ✓ ssh (BatchMode)\n")
	if r.MPVersion == "missing" {
		fmt.Fprintf(w, "  ✗ mp: not found on PATH or ~/.local/bin (install it there)\n")
	} else if !r.VersionMatch {
		fmt.Fprintf(w, "  ⚠ mp %s (local is %s — remote flags/schemas win)\n", r.MPVersion, r.LocalVersion)
	} else {
		fmt.Fprintf(w, "  ✓ mp %s = local\n", r.MPVersion)
	}
	fmt.Fprintf(w, "  %s git  %s tmux  %s gh", tick(r.Git), tick(r.Tmux), tick(r.Gh))
	if r.Gh {
		fmt.Fprintf(w, " (auth: %s)", tick(r.GhAuth))
	}
	fmt.Fprintln(w)
}
