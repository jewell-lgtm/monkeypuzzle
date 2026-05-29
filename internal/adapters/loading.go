package adapters

import (
	"fmt"
	"io"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// NewCLILoadingSubscriber returns a subscriber that prints loading messages to the writer.
// Use for CLI/text mode output.
func NewCLILoadingSubscriber(w io.Writer) func(active bool, label string) {
	return func(active bool, label string) {
		if active && label != "" {
			_, _ = fmt.Fprintf(w, "⏳ %s\n", label)
		}
	}
}

// NewNoopLoadingSubscriber returns a subscriber that does nothing.
// Use for JSON mode where loading state is not displayed.
func NewNoopLoadingSubscriber() func(active bool, label string) {
	return func(active bool, label string) {}
}

// SetupCLILoading creates a LoadingSignal with CLI subscriber attached.
func SetupCLILoading(w io.Writer) *core.LoadingSignal {
	loading := core.NewLoadingSignal()
	loading.Sub(NewCLILoadingSubscriber(w))
	return loading
}

// SetupNoopLoading creates a LoadingSignal with no-op subscriber.
func SetupNoopLoading() *core.LoadingSignal {
	loading := core.NewLoadingSignal()
	loading.Sub(NewNoopLoadingSubscriber())
	return loading
}
