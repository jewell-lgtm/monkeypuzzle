package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Remote execution proxy: `mp --host <ssh-host> [--dir <path>] <cmd> ...`
// forwards the whole invocation over ssh to an `mp` binary on the remote
// host, where the repo and worktrees live. Nothing in core runs locally, so
// worktree paths, hooks, and forge auth all resolve on the remote machine.
//
// The proxy flags are extracted from argv *before* cobra parses anything,
// and only from the stretch between `mp` and the first non-flag token (the
// verb): `mp --host wire list`, never `mp list --host wire`. That keeps
// forwarding lossless under version skew (flags only a newer remote mp
// understands never fail local parsing) and keeps verbs that own flags with
// the same names (`mp init --dir`) untouched.

// remoteTarget is an ssh destination for proxied execution.
type remoteTarget struct {
	host    string
	dir     string // remote working directory; empty = ssh login dir
	fromEnv bool   // target came from MP_HOST, not an explicit flag
}

// localOnlyVerbs never proxy, even with MP_HOST exported: completion and help
// are about the local binary, and `mp remote` (doctor etc.) probes hosts
// itself.
var localOnlyVerbs = map[string]bool{
	"__complete": true, "__completeNoDesc": true, "completion": true,
	"help": true, "remote": true,
}

// extractRemoteTarget strips --host/--dir (and = forms) from the leading
// flags of argv — extraction stops at the first token that isn't a proxy
// flag — and returns the remaining args plus the target, if any. Falls back
// to MP_HOST/MP_DIR when the flags are absent. A --dir without a host is an
// error: mp never proxies based on cwd or a bare directory alone.
func extractRemoteTarget(args []string) ([]string, *remoteTarget, error) {
	var host, dir string
	dirFromFlag := false
	i := 0
scan:
	for i < len(args) {
		arg := args[i]
		var name, value string
		var hasValue bool
		switch {
		case arg == "--host" || arg == "--dir":
			name = arg
		case strings.HasPrefix(arg, "--host="), strings.HasPrefix(arg, "--dir="):
			name, value, _ = strings.Cut(arg, "=")
			hasValue = true
		default:
			break scan
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag needs an argument: %s", name)
			}
			i++
			value = args[i]
		}
		i++
		if name == "--host" {
			host = value
		} else {
			dir = value
			dirFromFlag = true
		}
	}
	rest := args[i:]

	// A --host past the verb would be silently swallowed by the help-only
	// persistent flag; refuse it loudly. (--dir stays verb-owned, e.g.
	// `mp init --dir`.)
	if host == "" {
		for _, a := range rest {
			if a == "--" {
				break
			}
			if a == "--host" || strings.HasPrefix(a, "--host=") {
				return nil, nil, fmt.Errorf("--host must come before the command (use: mp --host <ssh-host> %s)", rest[0])
			}
		}
	}

	fromEnv := false
	if host == "" && os.Getenv("MP_HOST") != "" {
		host = os.Getenv("MP_HOST")
		fromEnv = true
	}
	if dir == "" {
		dir = os.Getenv("MP_DIR")
	}

	if host == "" || (len(rest) > 0 && localOnlyVerbs[rest[0]]) {
		if dirFromFlag && host == "" {
			return nil, nil, fmt.Errorf("--dir requires --host (mp never proxies from a directory alone)")
		}
		// A stray MP_DIR without a host is ignored rather than fatal.
		return rest, nil, nil
	}
	if err := validSSHDest(host); err != nil {
		return nil, nil, err
	}
	return rest, &remoteTarget{host: host, dir: dir, fromEnv: fromEnv}, nil
}

// validSSHDest rejects ssh destinations that could be parsed as ssh options
// or smuggle shell syntax — a registry file is user-writable, so a poisoned
// host must fail closed rather than reach ssh's argv.
func validSSHDest(host string) error {
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\n'\"`$;|&\\") {
		return fmt.Errorf("invalid ssh host %q", host)
	}
	return nil
}

// remoteBin is the mp binary to run on the remote host; MP_REMOTE_BIN (local
// env) overrides the default PATH lookup.
func remoteBin() string {
	if bin := os.Getenv("MP_REMOTE_BIN"); bin != "" {
		return bin
	}
	return "mp"
}

// remoteCommand builds the shell command executed on the remote host, always
// wrapped in `sh -c` so it runs under a POSIX shell regardless of the remote
// user's login shell (sshd hands the command string to fish/csh verbatim).
// Non-interactive ssh gets a bare sshd PATH that omits ~/.local/bin — mp's
// own install location — so it is prepended before the lookup.
func remoteCommand(target *remoteTarget, args []string) string {
	var b strings.Builder
	b.WriteString(`export PATH="$HOME/.local/bin:$PATH"; `)
	if target.dir != "" {
		b.WriteString("cd " + cli.ShQuote(target.dir) + " && ")
	}
	b.WriteString("exec " + cli.ShQuote(remoteBin()))
	for _, a := range args {
		b.WriteString(" " + cli.ShQuote(a))
	}
	return "sh -c " + cli.ShQuote(b.String())
}

// sshArgv builds the full local ssh invocation. BatchMode keeps agents from
// hanging on password prompts; ConnectTimeout bounds unreachable hosts. A
// remote pty (-t) is requested only when pty is true — the caller decides
// once, so pty allocation and stdin handling can never disagree.
func sshArgv(target *remoteTarget, args []string, pty bool) []string {
	argv := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}
	if pty {
		argv = append(argv, "-t")
	}
	argv = append(argv, "--", target.host, remoteCommand(target, args))
	return argv
}

// runRemote executes the proxied command and returns its exit code.
func runRemote(target *remoteTarget, args []string) int {
	if target.fromEnv {
		fmt.Fprintf(os.Stderr, "mp: forwarding to %s (MP_HOST is set)\n", target.host)
	}
	// A remote pty is allocated only when the local invocation is fully
	// interactive, so piped/JSON usage stays byte-clean.
	pty := cli.IsTerminal() && cli.IsStdoutTerminal()
	cmdArgv := sshArgv(target, args, pty)
	cmd := exec.Command(cmdArgv[0], cmdArgv[1:]...)
	cmd.Stdin = os.Stdin
	// Without a pty the remote mp always sees a pipe for stdin, so it takes
	// the stdin-JSON path even when the local caller supplied nothing — and a
	// zero-byte read is "invalid JSON", while a local tty as stdin would make
	// the remote block forever waiting for EOF. Substitute the empty JSON
	// object: same meaning ("no input, use defaults"), valid to parse.
	if !pty && !cli.HasStdinData() {
		cmd.Stdin = strings.NewReader("{}")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// ssh itself exits 255 on connection failure; anything else is
			// the remote mp's own exit code, passed through verbatim.
			if exitErr.ExitCode() == 255 {
				fmt.Fprintf(os.Stderr, "mp: ssh to %s failed (check `ssh %s` works and mp is installed there)\n", target.host, target.host)
			}
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "mp: %v\n", err)
		return 1
	}
	return 0
}
