package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

func TestExtractRemoteTarget(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantRest []string
		wantTgt  *remoteTarget
		wantErr  string
	}{
		{
			name:     "no host, args untouched",
			args:     []string{"create", "--prompt", "add rate limiting"},
			wantRest: []string{"create", "--prompt", "add rate limiting"},
		},
		{
			name:     "host and dir flags stripped",
			args:     []string{"--host", "wire", "--dir", "/home/u/api", "list", "--json"},
			wantRest: []string{"list", "--json"},
			wantTgt:  &remoteTarget{host: "wire", dir: "/home/u/api"},
		},
		{
			name:     "equals form",
			args:     []string{"--host=wire", "--dir=/x", "create"},
			wantRest: []string{"create"},
			wantTgt:  &remoteTarget{host: "wire", dir: "/x"},
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
			name:     "verb-owned --dir stays with the verb",
			args:     []string{"init", "--dir", "/x"},
			wantRest: []string{"init", "--dir", "/x"},
		},
		{
			name:    "leading dir without host errors",
			args:    []string{"--dir", "/x", "list"},
			wantErr: "--dir requires --host",
		},
		{
			name:    "host missing value errors",
			args:    []string{"--host"},
			wantErr: "flag needs an argument",
		},
		{
			name:    "option-shaped host is rejected",
			args:    []string{"--host", "-oProxyCommand=evil", "list"},
			wantErr: "invalid ssh host",
		},
		{
			name:     "MP_HOST env drives forwarding",
			args:     []string{"go", "--json"},
			env:      map[string]string{"MP_HOST": "wire", "MP_DIR": "/srv/api"},
			wantRest: []string{"go", "--json"},
			wantTgt:  &remoteTarget{host: "wire", dir: "/srv/api", fromEnv: true},
		},
		{
			name:     "flag beats env",
			args:     []string{"--host", "other", "list"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"list"},
			wantTgt:  &remoteTarget{host: "other"},
		},
		{
			name:     "stray MP_DIR without any host is ignored",
			args:     []string{"list"},
			env:      map[string]string{"MP_DIR": "/x"},
			wantRest: []string{"list"},
		},
		{
			name:     "completion never proxies even with MP_HOST",
			args:     []string{"__complete", "li"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"__complete", "li"},
		},
		{
			name:     "help never proxies even with MP_HOST",
			args:     []string{"help", "create"},
			env:      map[string]string{"MP_HOST": "wire"},
			wantRest: []string{"help", "create"},
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
			rest, tgt, err := extractRemoteTarget(tt.args)
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
			if !reflect.DeepEqual(tgt, tt.wantTgt) {
				t.Errorf("target = %+v, want %+v", tgt, tt.wantTgt)
			}
		})
	}
}

func TestValidSSHDest(t *testing.T) {
	for _, ok := range []string{"wire", "user@wire", "wire.example.com", "10.0.0.7", "box_1"} {
		if err := validSSHDest(ok); err != nil {
			t.Errorf("validSSHDest(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=x", "host name", "host;rm", "host`x`", "a'b", `a"b`, "a$b", "a|b", "a&b", "a\\b"} {
		if err := validSSHDest(bad); err == nil {
			t.Errorf("validSSHDest(%q) = nil, want error", bad)
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
