package main

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// resolveApply decides whether a dry-run-by-default command should apply its
// changes. The contract:
//
//   - --apply (applyFlag) forces the mutation; --dry-run (dryRunFlag) forces a
//     preview. Passing both is contradictory and fails loudly.
//   - With neither flag, an interactive terminal (real TTY, no piped stdin) is
//     prompted to confirm — but only when there is something to do.
//   - Any non-interactive invocation (agent/JSON/redirected) defaults to a
//     preview, never mutating without an explicit opt-in.
//
// confirm shows the interactive prompt after the preview has been printed; it
// is only invoked in the interactive branch.
func resolveApply(applyFlag, dryRunFlag, anythingToDo bool, confirm func() (bool, error)) (bool, error) {
	if applyFlag && dryRunFlag {
		return false, fmt.Errorf("cannot use --apply and --dry-run together")
	}
	if applyFlag {
		return true, nil
	}
	if dryRunFlag {
		return false, nil
	}
	if cli.IsTerminal() && !cli.HasStdinData() && anythingToDo {
		return confirm()
	}
	return false, nil
}

// confirmApply prompts the user to apply the previewed changes. It returns true
// only if they explicitly choose to apply; cancelling leaves the dry-run intact.
func confirmApply(title, summary string) (bool, error) {
	choice, ok, err := chooser.Run(
		title,
		[]string{summary},
		[]chooser.Option{
			{Label: "Apply", Desc: "make the changes previewed above", Value: "apply"},
			{Label: "Cancel", Desc: "leave everything unchanged (dry-run only)", Value: ""},
		},
	)
	if err != nil {
		return false, err
	}
	return ok && choice == "apply", nil
}
