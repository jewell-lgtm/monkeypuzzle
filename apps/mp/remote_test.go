package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestShQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"/home/matt/code/api", "'/home/matt/code/api'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"'; rm -rf /; '", `''\''; rm -rf /; '\'''`},
		{"a$b`c", "'a$b`c'"},
		{"multi\nline", "'multi\nline'"},
		{"~/code", "'~/code'"}, // tilde does NOT expand inside quotes -- --dir must be absolute
	}
	for _, tt := range tests {
		if got := shQuote(tt.in); got != tt.want {
			t.Errorf("shQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

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
			args:     []string{"create", "--host=wire", "--dir=/x"},
			wantRest: []string{"create"},
			wantTgt:  &remoteTarget{host: "wire", dir: "/x"},
		},
		{
			name:     "flag value that merely contains --host is not stripped",
			args:     []string{"create", "--prompt", "--host is a flag"},
			wantRest: []string{"create", "--prompt", "--host is a flag"},
		},
		{
			name:    "dir without host errors",
			args:    []string{"--dir", "/x", "list"},
			wantErr: "--dir requires --host",
		},
		{
			name:    "host missing value errors",
			args:    []string{"list", "--host"},
			wantErr: "flag needs an argument",
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

func TestRemoteCommand(t *testing.T) {
	t.Setenv("MP_REMOTE_BIN", "")
	pathPrefix := `export PATH="$HOME/.local/bin:$PATH"; `

	tgt := &remoteTarget{host: "wire", dir: "/home/u/my repo"}
	got := remoteCommand(tgt, []string{"create", "--prompt", "fix the o'clock bug", "--json"})
	want := pathPrefix + `cd '/home/u/my repo' && exec 'mp' 'create' '--prompt' 'fix the o'\''clock bug' '--json'`
	if got != want {
		t.Errorf("remoteCommand = %s\nwant            %s", got, want)
	}

	noDir := &remoteTarget{host: "wire"}
	if got := remoteCommand(noDir, []string{"go", "--json"}); got != pathPrefix+`exec 'mp' 'go' '--json'` {
		t.Errorf("remoteCommand without dir = %s", got)
	}

	t.Setenv("MP_REMOTE_BIN", "/opt/mp/bin/mp")
	if got := remoteCommand(noDir, []string{"list"}); got != pathPrefix+`exec '/opt/mp/bin/mp' 'list'` {
		t.Errorf("remoteCommand with MP_REMOTE_BIN = %s", got)
	}
}
