package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

func TestExtractRemoteSpec(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantRest []string
		wantSpec remoteSpec
		wantErr  string
	}{
		{
			name:     "no proxy flags, args untouched",
			args:     []string{"create", "--prompt", "add rate limiting"},
			wantRest: []string{"create", "--prompt", "add rate limiting"},
		},
		{
			name:     "host and dir flags stripped",
			args:     []string{"--host", "wire", "--dir", "/home/u/api", "list", "--json"},
			wantRest: []string{"list", "--json"},
			wantSpec: remoteSpec{host: "wire", dir: "/home/u/api", dirFromFlag: true},
		},
		{
			name:     "equals form with project",
			args:     []string{"--host=wire", "--project=api", "create"},
			wantRest: []string{"create"},
			wantSpec: remoteSpec{host: "wire", project: "api"},
		},
		{
			name:     "project alone",
			args:     []string{"--project", "api", "create", "--json"},
			wantRest: []string{"create", "--json"},
			wantSpec: remoteSpec{project: "api"},
		},
		{
			name:    "host after the verb is refused, not swallowed",
			args:    []string{"create", "--host", "wire"},
			wantErr: "--host must come before the command",
		},
		{
			name:     "host after -- is positional territory",
			args:     []string{"run", "--", "--host"},
			wantRest: []string{"run", "--", "--host"},
		},
		{
			name:     "verb-owned --dir and --project stay with the verb",
			args:     []string{"switch", "--project", "api", "--piece", "x"},
			wantRest: []string{"switch", "--project", "api", "--piece", "x"},
		},
		{
			name:    "flag missing value errors",
			args:    []string{"--project"},
			wantErr: "flag needs an argument",
		},
		{
			name:     "MP_HOST env drives forwarding",
			args:     []string{"go", "--json"},
			env:      map[string]string{"MP_HOST": "wire", "MP_DIR": "/srv/api"},
			wantRest: []string{"go", "--json"},
			wantSpec: remoteSpec{host: "wire", dir: "/srv/api", hostFromEnv: true},
		},
		{
			name:     "flag beats env",
			args:     []string{"--host", "other", "list"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"list"},
			wantSpec: remoteSpec{host: "other"},
		},
		{
			name:     "completion never proxies even with MP_HOST",
			args:     []string{"__complete", "li"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"__complete", "li"},
		},
		{
			name:     "remote doctor never proxies even with MP_HOST",
			args:     []string{"remote", "doctor"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"remote", "doctor"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, ok := tt.env["MP_HOST"]; !ok {
				t.Setenv("MP_HOST", "")
			}
			if _, ok := tt.env["MP_DIR"]; !ok {
				t.Setenv("MP_DIR", "")
			}
			rest, spec, err := extractRemoteSpec(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(rest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
			if spec != tt.wantSpec {
				t.Errorf("spec = %+v, want %+v", spec, tt.wantSpec)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MP_DATA_DIR", dataDir)
	reg := `{"version":"1","projects":[
		{"name":"api","path":"/home/u/api","host":"wire","added_at":"2026-01-01T00:00:00Z"},
		{"name":"web","path":"/local/web","added_at":"2026-01-01T00:00:00Z"},
		{"name":"dup","path":"/local/dup","added_at":"2026-01-01T00:00:00Z"},
		{"name":"dup","path":"/home/u/dup","host":"wire","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dataDir, "projects.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		spec      remoteSpec
		wantTgt   *remoteTarget
		wantChdir string
		wantErr   string
	}{
		{name: "empty spec is a plain local run", spec: remoteSpec{}},
		{
			name:    "remote project proxies to its host+path",
			spec:    remoteSpec{project: "api"},
			wantTgt: &remoteTarget{host: "wire", dir: "/home/u/api"},
		},
		{
			name:      "local project resolves to chdir",
			spec:      remoteSpec{project: "web"},
			wantChdir: "/local/web",
		},
		{
			name:    "explicit host wins, project supplies dir",
			spec:    remoteSpec{host: "other", project: "api"},
			wantTgt: &remoteTarget{host: "other", dir: "/home/u/api"},
		},
		{
			name:    "--dir overrides the project path when proxying",
			spec:    remoteSpec{project: "api", dir: "/elsewhere", dirFromFlag: true},
			wantTgt: &remoteTarget{host: "wire", dir: "/elsewhere"},
		},
		{
			name:    "--dir with a local project errors",
			spec:    remoteSpec{project: "web", dir: "/x", dirFromFlag: true},
			wantErr: "--dir requires a remote target",
		},
		{
			name:    "--dir alone errors",
			spec:    remoteSpec{dir: "/x", dirFromFlag: true},
			wantErr: "--dir requires a remote target",
		},
		{
			name: "stray MP_DIR without a host is ignored",
			spec: remoteSpec{dir: "/x"},
		},
		{
			name:    "ambiguous name refuses to guess a machine",
			spec:    remoteSpec{project: "dup"},
			wantErr: "ambiguous",
		},
		{
			name:    "host:path disambiguates",
			spec:    remoteSpec{project: "wire:/home/u/dup"},
			wantTgt: &remoteTarget{host: "wire", dir: "/home/u/dup"},
		},
		{
			name:      "plain path disambiguates to local",
			spec:      remoteSpec{project: "/local/dup"},
			wantChdir: "/local/dup",
		},
		{
			name:    "unknown project errors",
			spec:    remoteSpec{project: "nope"},
			wantErr: "no registered project",
		},
		{
			name:    "option-shaped host is rejected",
			spec:    remoteSpec{host: "-oProxyCommand=evil"},
			wantErr: "invalid ssh host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tgt, chdir, err := resolveTarget(tt.spec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(tgt, tt.wantTgt) {
				t.Errorf("target = %+v, want %+v", tgt, tt.wantTgt)
			}
			if chdir != tt.wantChdir {
				t.Errorf("chdir = %q, want %q", chdir, tt.wantChdir)
			}
		})
	}
}

func TestSplitHostPath(t *testing.T) {
	tests := []struct {
		in, host, path string
	}{
		{"/local/path", "", "/local/path"},
		{"relative/dir", "", "relative/dir"},
		{"wire:/home/u/api", "wire", "/home/u/api"},
		{"wire:code/api", "wire", "code/api"},
		{"user@wire:code", "user@wire", "code"},
		{":/weird", "", ":/weird"},
		{"./a:b", "", "./a:b"}, // colon after a slash: a local path, not a host
	}
	for _, tt := range tests {
		host, path := splitHostPath(tt.in)
		if host != tt.host || path != tt.path {
			t.Errorf("splitHostPath(%q) = (%q, %q), want (%q, %q)", tt.in, host, path, tt.host, tt.path)
		}
	}
}

func TestRemoteCommand(t *testing.T) {
	t.Setenv("MP_REMOTE_BIN", "")
	inner := func(s string) string { return "sh -c " + cli.ShQuote(s) }
	pathPrefix := `export PATH="$HOME/.local/bin:$PATH"; `

	tgt := &remoteTarget{host: "wire", dir: "/home/u/my repo"}
	got := remoteCommand(tgt, []string{"create", "--prompt", "fix the o'clock bug", "--json"})
	want := inner(pathPrefix + `cd '/home/u/my repo' && exec 'mp' 'create' '--prompt' 'fix the o'\''clock bug' '--json'`)
	if got != want {
		t.Errorf("remoteCommand = %s\nwant            %s", got, want)
	}

	noDir := &remoteTarget{host: "wire"}
	if got := remoteCommand(noDir, []string{"go", "--json"}); got != inner(pathPrefix+`exec 'mp' 'go' '--json'`) {
		t.Errorf("remoteCommand without dir = %s", got)
	}

	t.Setenv("MP_REMOTE_BIN", "/opt/mp/bin/mp")
	if got := remoteCommand(noDir, []string{"list"}); got != inner(pathPrefix+`exec '/opt/mp/bin/mp' 'list'`) {
		t.Errorf("remoteCommand with MP_REMOTE_BIN = %s", got)
	}
}

// D4: placement calls tell the box-side mp who placed it; plain proxying
// exports nothing, and MP_HOST (the reroute var) never appears.
func TestRemoteCommand_PlacementExports(t *testing.T) {
	t.Setenv("MP_REMOTE_BIN", "")
	inner := func(s string) string { return "sh -c " + cli.ShQuote(s) }

	placed := &remoteTarget{host: "wire", dir: "/x", placement: true}
	got := remoteCommand(placed, []string{"status"})
	want := inner(`export PATH="$HOME/.local/bin:$PATH"; export MP_PLACEMENT_HOST='wire' MP_REMOTE=1; cd '/x' && exec 'mp' 'status'`)
	if got != want {
		t.Errorf("placement remoteCommand = %s\nwant %s", got, want)
	}

	plain := &remoteTarget{host: "wire", dir: "/x"}
	if got := remoteCommand(plain, []string{"status"}); strings.Contains(got, "MP_PLACEMENT_HOST") || strings.Contains(got, "MP_REMOTE") {
		t.Errorf("plain proxy must not export placement vars: %s", got)
	}

	for _, tgt := range []*remoteTarget{placed, plain, {host: "wire", fromEnv: true, placement: true}} {
		for _, a := range sshArgv(tgt, []string{"create", "--name", "p"}, false) {
			if strings.Contains(a, "MP_HOST") {
				t.Errorf("MP_HOST leaked into ssh argv: %q", a)
			}
		}
	}
}

func TestSSHArgv(t *testing.T) {
	t.Setenv("MP_REMOTE_BIN", "")
	tgt := &remoteTarget{host: "wire", dir: "/x"}

	got := sshArgv(tgt, []string{"list"}, false)
	want := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "--", "wire", remoteCommand(tgt, []string{"list"})}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sshArgv(pty=false) = %v, want %v", got, want)
	}

	withPty := sshArgv(tgt, []string{"list"}, true)
	if withPty[5] != "-t" || len(withPty) != len(want)+1 {
		t.Errorf("sshArgv(pty=true) = %v, want -t inserted before --", withPty)
	}
}

func TestStripSelector(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"status", "fix", "--json"}, "status --json"},
		{[]string{"done", "--piece", "fix"}, "done"},
		{[]string{"done", "--piece=fix", "--main", "trunk"}, "done --main trunk"},
		{[]string{"abandon", "--name", "fix", "--force"}, "abandon --force"},
		{[]string{"abandon", "--name", "other", "fix"}, "abandon --name other"},
	}
	for _, c := range cases {
		if got := strings.Join(stripSelector(c.in, "fix"), " "); got != c.want {
			t.Errorf("stripSelector(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
