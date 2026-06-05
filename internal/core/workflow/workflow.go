// Package workflow models per-project issue lifecycles. A Workflow declares
// named states, transitions between them driven by named events, and a
// per-provider mapping from workflow state names to native provider state
// identifiers. The engine is pure logic: it resolves "given current state +
// event, what's the next state?" and does not touch git, providers, or disk.
package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Event names this engine knows about. The list is open — workflows may
// declare their own custom events — but these constants cover the events
// monkeypuzzle understands as standard.
//
// Today only branch.created and pr.merged are observed automatically (by
// `mp piece create` and `mp piece merge` respectively). The PR-state and
// check-status events are reserved for the follow-up tracked at
// issues/observe-github-pr-events-for-workflow.md; users can still drive
// them manually with `mp issue fire --event <name>`.
const (
	EventBranchCreated    = "branch.created"
	EventPROpenedDraft    = "pr.opened.draft"
	EventPROpenedReady    = "pr.opened.ready"
	EventPRChecksGreen    = "pr.checks.green"
	EventPRPreviewReady   = "pr.preview.ready"
	EventPRMerged         = "pr.merged"
	EventPRClosedUnmerged = "pr.closed_unmerged"
	EventAcceptancePassed = "acceptance.passed"
	EventReleased         = "released"
	EventSmokePassed      = "smoke.passed"
	EventAbandoned        = "abandoned"
)

// FromAny is the wildcard "from" state used by transitions that fire from
// any state (e.g. abandoned).
const FromAny = "*"

// CancelStateDefault is the cancel state used when a workflow doesn't name one.
const CancelStateDefault = "cancelled"

// Workflow is the declarative shape stored in monkeypuzzle.json under the
// top-level "workflow" key.
type Workflow struct {
	States      []string                    `json:"states"`
	Initial     string                      `json:"initial"`
	Terminal    []string                    `json:"terminal,omitempty"`
	Cancel      CancelSpec                  `json:"cancel"`
	Transitions []Transition                `json:"transitions"`
	ProviderMap map[string]ProviderMapEntry `json:"provider_map,omitempty"`
}

// CancelSpec describes the cancel axis. When FromAny is true, the abandoned
// event can fire from any state; otherwise the workflow must declare each
// cancel transition explicitly in Transitions.
type CancelSpec struct {
	State   string `json:"state"`
	FromAny bool   `json:"from_any"`
}

// Transition is one edge in the state graph.
//
// From may be "*" (FromAny) to mean "any state". On is the name of the event
// that fires this transition. Requires lists extra events whose latest fire
// must be observed before On is considered satisfied — used by workflows
// that need to AND together signals (e.g. checks green AND preview ready).
type Transition struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	On       string   `json:"on"`
	Requires []string `json:"requires,omitempty"`
}

// ProviderMapEntry binds one workflow state to a provider-native identifier.
// Different providers fill in different fields:
//
//   - Plane uses StateID (a Plane state UUID) or Group (a Plane state group
//     name, used by the default workflow for backwards compatibility).
//   - Markdown uses Frontmatter (the literal value written to the status:
//     frontmatter field).
type ProviderMapEntry struct {
	StateID     string `json:"state_id,omitempty"`
	Group       string `json:"group,omitempty"`
	Frontmatter string `json:"frontmatter,omitempty"`
}

// Default returns the built-in workflow used when a project's config has no
// workflow block. Behavior matches monkeypuzzle's pre-RFC defaults exactly,
// plus a `cancelled` state distinct from `done`.
func Default() Workflow {
	return Workflow{
		States:   []string{"todo", "in-progress", "done"},
		Initial:  "todo",
		Terminal: []string{"done"},
		Cancel:   CancelSpec{State: "cancelled", FromAny: true},
		Transitions: []Transition{
			{From: "todo", To: "in-progress", On: EventBranchCreated},
			{From: "in-progress", To: "done", On: EventPRMerged},
			{From: FromAny, To: "cancelled", On: EventAbandoned},
		},
		ProviderMap: map[string]ProviderMapEntry{
			// markdown round-trips workflow state names verbatim.
			"markdown.todo":        {Frontmatter: "todo"},
			"markdown.in-progress": {Frontmatter: "in-progress"},
			"markdown.done":        {Frontmatter: "done"},
			"markdown.cancelled":   {Frontmatter: "cancelled"},
			// plane uses state groups for the default workflow so existing
			// projects keep their pre-RFC behavior.
			"plane.todo":        {Group: "backlog"},
			"plane.in-progress": {Group: "started"},
			"plane.done":        {Group: "completed"},
			"plane.cancelled":   {Group: "cancelled"},
		},
	}
}

// Validate reports whether the workflow is internally consistent. It is the
// caller's responsibility to run this on user-supplied workflows; the engine
// itself trusts the workflow it's handed.
func (w Workflow) Validate() error {
	if len(w.States) == 0 {
		return fmt.Errorf("workflow.states is empty")
	}
	if w.Initial == "" {
		return fmt.Errorf("workflow.initial is empty")
	}
	known := map[string]bool{}
	for _, s := range w.States {
		known[s] = true
	}
	known[w.Cancel.State] = true
	if !known[w.Initial] {
		return fmt.Errorf("workflow.initial %q is not in workflow.states", w.Initial)
	}
	for _, t := range w.Terminal {
		if !known[t] {
			return fmt.Errorf("workflow.terminal contains unknown state %q", t)
		}
	}
	for i, tr := range w.Transitions {
		if tr.From != FromAny && !known[tr.From] {
			return fmt.Errorf("transitions[%d].from %q is not in workflow.states", i, tr.From)
		}
		if !known[tr.To] {
			return fmt.Errorf("transitions[%d].to %q is not in workflow.states", i, tr.To)
		}
		if tr.On == "" {
			return fmt.Errorf("transitions[%d].on is empty", i)
		}
	}
	return nil
}

// AllStates returns States plus the cancel state if it isn't already present.
// Useful for any UI or filter that wants the complete set of legal values.
func (w Workflow) AllStates() []string {
	out := append([]string{}, w.States...)
	if w.Cancel.State != "" {
		found := false
		for _, s := range out {
			if s == w.Cancel.State {
				found = true
				break
			}
		}
		if !found {
			out = append(out, w.Cancel.State)
		}
	}
	return out
}

// HasState reports whether the workflow includes the given state name
// (treating the cancel state as a member of the workflow).
func (w Workflow) HasState(name string) bool {
	for _, s := range w.AllStates() {
		if s == name {
			return true
		}
	}
	return false
}

// IsTerminal reports whether the given state is listed in Terminal or is the
// cancel state.
func (w Workflow) IsTerminal(state string) bool {
	if state == w.Cancel.State {
		return true
	}
	for _, t := range w.Terminal {
		if t == state {
			return true
		}
	}
	return false
}

// ProviderEntry returns the per-provider mapping for the given (provider,
// state) pair. ok is false when no mapping is declared.
func (w Workflow) ProviderEntry(provider, state string) (ProviderMapEntry, bool) {
	if w.ProviderMap == nil {
		return ProviderMapEntry{}, false
	}
	entry, ok := w.ProviderMap[provider+"."+state]
	return entry, ok
}

// FireResult describes what changed when an event was applied.
type FireResult struct {
	From string
	To   string
	On   string
}

// Fire applies an event to the current state, returning the resolved next
// state (and the transition that fired). If no transition matches, returns
// (current, false, nil).
//
// The caller is responsible for "requires" satisfaction — Fire treats every
// transition's Requires field as already-satisfied. Callers that need to
// AND multiple events together (e.g. checks-green AND preview-ready before
// transitioning) must track those signals separately and only call Fire
// when all are present.
func (w Workflow) Fire(current, event string) (FireResult, bool) {
	// Prefer specific (from == current) over wildcard (from == "*") so that
	// `abandoned` from a known state still uses any state-specific cancel
	// transition the workflow declared.
	var wild *Transition
	for i := range w.Transitions {
		tr := w.Transitions[i]
		if tr.On != event {
			continue
		}
		if tr.From == current {
			return FireResult{From: current, To: tr.To, On: tr.On}, true
		}
		if tr.From == FromAny && wild == nil {
			wild = &tr
		}
	}
	if wild != nil {
		return FireResult{From: current, To: wild.To, On: wild.On}, true
	}
	return FireResult{From: current, To: current, On: event}, false
}

// OutboundEvents returns the events that can fire from the given state,
// sorted for stable output. Wildcard transitions are included.
func (w Workflow) OutboundEvents(current string) []string {
	seen := map[string]bool{}
	for _, tr := range w.Transitions {
		if tr.From == current || tr.From == FromAny {
			seen[tr.On] = true
		}
	}
	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// IsManualEvent reports whether the named event must be fired explicitly
// (i.e. not derivable from git/PR state). The list mirrors §2.2 of the RFC.
func IsManualEvent(event string) bool {
	switch event {
	case EventAcceptancePassed, EventReleased, EventSmokePassed, EventAbandoned:
		return true
	default:
		return false
	}
}

// OutboundManualEvents returns the subset of OutboundEvents that are manual.
func (w Workflow) OutboundManualEvents(current string) []string {
	out := make([]string, 0)
	for _, e := range w.OutboundEvents(current) {
		if IsManualEvent(e) {
			out = append(out, e)
		}
	}
	return out
}

// ParseJSON parses a Workflow from a raw JSON blob (as found inside
// monkeypuzzle.json's "workflow" key). Returns the default workflow if the
// blob is empty.
func ParseJSON(data []byte) (Workflow, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return Default(), nil
	}
	var w Workflow
	if err := json.Unmarshal(data, &w); err != nil {
		return Workflow{}, fmt.Errorf("invalid workflow JSON: %w", err)
	}
	if w.Cancel.State == "" {
		w.Cancel.State = CancelStateDefault
	}
	return w, nil
}
