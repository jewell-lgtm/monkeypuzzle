// Package history keeps an append-only JSONL log of everything mp did, across
// every repository on the machine. It is a write-behind audit trail: recording
// an event must never fail the verb that produced it, so the convenience entry
// point (Record) downgrades every error to a warning.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// EnvFile overrides the log file path (tests, custom setups).
const EnvFile = "MP_HISTORY_FILE"

// ErrNoHome is returned when neither MP_HISTORY_FILE, XDG_STATE_HOME nor HOME
// can locate the log.
var ErrNoHome = errors.New("history: cannot resolve log path (set MP_HISTORY_FILE, XDG_STATE_HOME or HOME)")

// Actor says who drove mp: a person at a terminal or an agent.
type Actor struct {
	Kind string `json:"kind"` // "user" | "agent"
	ID   string `json:"id,omitempty"`
}

// Event is one line of the log.
type Event struct {
	TS      string         `json:"ts"` // RFC3339 UTC
	Event   string         `json:"event"`
	Project string         `json:"project"`
	Piece   string         `json:"piece"`
	Branch  string         `json:"branch,omitempty"`
	Parent  string         `json:"parent,omitempty"`
	Host    string         `json:"host,omitempty"`
	Actor   Actor          `json:"actor"`
	Data    map[string]any `json:"data,omitempty"`
}

// Time parses the event timestamp; zero when unparseable.
func (e Event) Time() time.Time {
	t, err := time.Parse(time.RFC3339, e.TS)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Path resolves the log file: $MP_HISTORY_FILE, else
// ${XDG_STATE_HOME:-$HOME/.local/state}/monkeypuzzle/history.jsonl.
func Path() (string, error) {
	if p := strings.TrimSpace(os.Getenv(EnvFile)); p != "" {
		return p, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", ErrNoHome
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "monkeypuzzle", "history.jsonl"), nil
}

// CurrentActor classifies the caller from the environment: an agent when
// CLAUDECODE (set by Claude Code) or MP_AGENT_ID is present, else a user.
func CurrentActor() Actor {
	id := os.Getenv("MP_AGENT_ID")
	if id != "" || os.Getenv("CLAUDECODE") != "" {
		return Actor{Kind: "agent", ID: id}
	}
	return Actor{Kind: "user"}
}

// Append writes ev as one JSON line. TS and Actor are filled in when empty.
// The file is opened O_APPEND and the line is written in a single call, so
// concurrent writers never interleave within a line.
func Append(ev Event) error {
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if ev.Actor.Kind == "" {
		ev.Actor = CurrentActor()
	}
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	_, werr := f.Write(append(line, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("history: %w", werr)
	}
	return nil
}

// Record appends ev and turns any failure into a warning on out (or stderr
// when out is nil). It never returns an error: the log must not fail a verb.
func Record(out core.Output, ev Event) {
	err := Append(ev)
	if err == nil {
		return
	}
	if out != nil {
		out.Write(core.Message{Type: core.MsgWarning, Content: err.Error()})
		return
	}
	fmt.Fprintln(os.Stderr, "Warning:", err)
}

// ReadOptions filters Read. Zero values mean "no filter".
type ReadOptions struct {
	Project string
	Piece   string
	// Event matches exactly, or by prefix with a trailing "*" ("pr.*").
	Event string
	Since time.Time
	// Limit keeps only the last N matching events (0 = all).
	Limit int
}

// Read returns matching events, oldest first. Malformed lines are skipped. A
// missing log reads as empty.
func Read(opts ReadOptions) ([]Event, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("history: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Event == "" {
			continue
		}
		if opts.matches(ev) {
			events = append(events, ev)
		}
	}
	if err := sc.Err(); err != nil {
		return events, fmt.Errorf("history: %w", err)
	}
	if opts.Limit > 0 && len(events) > opts.Limit {
		events = events[len(events)-opts.Limit:]
	}
	return events, nil
}

func (o ReadOptions) matches(ev Event) bool {
	if o.Project != "" && ev.Project != o.Project {
		return false
	}
	if o.Piece != "" && ev.Piece != o.Piece {
		return false
	}
	if o.Event != "" && !MatchEvent(o.Event, ev.Event) {
		return false
	}
	if !o.Since.IsZero() && ev.Time().Before(o.Since) {
		return false
	}
	return true
}

// MatchEvent reports whether name matches pattern: exact, or prefix when the
// pattern ends in "*" ("pr.*" matches "pr.created"; "*" matches everything).
func MatchEvent(pattern, name string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(name, prefix)
	}
	return pattern == name
}
