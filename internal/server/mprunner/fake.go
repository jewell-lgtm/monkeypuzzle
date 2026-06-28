package mprunner

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// Fake is an in-memory MpRunner for tests. It records every call and returns
// the canned Stacks/Err.
type Fake struct {
	Stacks []stackgraph.Stack
	Err    error
	Calls  []StackGraphInput
}

// StackGraph records in and returns the canned result.
func (f *Fake) StackGraph(_ context.Context, in StackGraphInput) ([]stackgraph.Stack, error) {
	f.Calls = append(f.Calls, in)
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Stacks, nil
}

var _ MpRunner = (*Fake)(nil)
