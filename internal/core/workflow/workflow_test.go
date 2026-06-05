package workflow

import "testing"

func TestDefault_Validates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default workflow should validate: %v", err)
	}
}

func TestDefault_BranchCreatedFires(t *testing.T) {
	w := Default()
	got, ok := w.Fire("todo", EventBranchCreated)
	if !ok {
		t.Fatal("expected branch.created to fire from todo")
	}
	if got.To != "in-progress" {
		t.Errorf("To = %q, want in-progress", got.To)
	}
}

func TestDefault_PRMergedFires(t *testing.T) {
	w := Default()
	got, ok := w.Fire("in-progress", EventPRMerged)
	if !ok {
		t.Fatal("expected pr.merged to fire from in-progress")
	}
	if got.To != "done" {
		t.Errorf("To = %q, want done", got.To)
	}
}

func TestDefault_AbandonedFromAnywhere(t *testing.T) {
	w := Default()
	for _, from := range []string{"todo", "in-progress", "done"} {
		got, ok := w.Fire(from, EventAbandoned)
		if !ok {
			t.Errorf("expected abandoned to fire from %s", from)
			continue
		}
		if got.To != "cancelled" {
			t.Errorf("from %s: To = %q, want cancelled", from, got.To)
		}
	}
}

func TestFire_UnknownEvent_NoChange(t *testing.T) {
	w := Default()
	got, ok := w.Fire("todo", "no.such.event")
	if ok {
		t.Errorf("unknown event should not fire; got transition to %q", got.To)
	}
	if got.To != "todo" {
		t.Errorf("To = %q, want todo (unchanged)", got.To)
	}
}

func TestFire_SpecificFromOverridesWildcard(t *testing.T) {
	w := Workflow{
		States:  []string{"a", "b", "c"},
		Initial: "a",
		Transitions: []Transition{
			{From: FromAny, To: "c", On: "x"},
			{From: "a", To: "b", On: "x"},
		},
	}
	got, ok := w.Fire("a", "x")
	if !ok {
		t.Fatal("expected x to fire from a")
	}
	if got.To != "b" {
		t.Errorf("specific-from should win; To = %q, want b", got.To)
	}
}

func TestValidate_CatchesUnknownState(t *testing.T) {
	w := Workflow{
		States: []string{"a"}, Initial: "a",
		Transitions: []Transition{{From: "a", To: "missing", On: "x"}},
	}
	if err := w.Validate(); err == nil {
		t.Error("expected error for unknown 'to' state")
	}
}

func TestProviderEntry(t *testing.T) {
	w := Default()
	if e, ok := w.ProviderEntry("markdown", "in-progress"); !ok || e.Frontmatter != "in-progress" {
		t.Errorf("markdown.in-progress: ok=%v entry=%+v", ok, e)
	}
	if e, ok := w.ProviderEntry("plane", "done"); !ok || e.Group != "completed" {
		t.Errorf("plane.done: ok=%v entry=%+v", ok, e)
	}
	if _, ok := w.ProviderEntry("linear", "todo"); ok {
		t.Error("linear.todo should not be in the default provider map")
	}
}

func TestParseJSON_EmptyReturnsDefault(t *testing.T) {
	for _, in := range []string{"", "  ", "null"} {
		w, err := ParseJSON([]byte(in))
		if err != nil {
			t.Errorf("ParseJSON(%q): unexpected error: %v", in, err)
		}
		if len(w.States) != 3 {
			t.Errorf("ParseJSON(%q): want default workflow, got states=%v", in, w.States)
		}
	}
}

func TestParseJSON_FillsCancelStateDefault(t *testing.T) {
	w, err := ParseJSON([]byte(`{"states":["a"],"initial":"a","transitions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Cancel.State != CancelStateDefault {
		t.Errorf("Cancel.State = %q, want %q", w.Cancel.State, CancelStateDefault)
	}
}

func TestOutboundManualEvents(t *testing.T) {
	w := Workflow{
		States:  []string{"a", "b"},
		Initial: "a",
		Transitions: []Transition{
			{From: "a", To: "b", On: EventPRMerged},
			{From: "a", To: "b", On: EventAcceptancePassed},
			{From: FromAny, To: "b", On: EventAbandoned},
		},
	}
	got := w.OutboundManualEvents("a")
	want := []string{EventAbandoned, EventAcceptancePassed}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q want %q", i, got[i], want[i])
		}
	}
}
