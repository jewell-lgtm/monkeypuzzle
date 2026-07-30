package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	agentcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var waitCmd = &cobra.Command{
	Use:   "wait [piece...]",
	Short: "Block until agents settle (no agent working)",
	Long: `Poll agent status until no agent in the target pieces is working — i.e. every
agent is blocked, done, or idle. Without arguments, waits on every piece that
has live agents. The fan-out pattern:

  mp create --name a --agent claude --prompt "..." --skip-switch
  mp create --name b --agent claude --prompt "..." --skip-switch
  mp wait && mp agent list

Exits 0 when settled (the JSON says which pieces are blocked vs done), non-zero
on timeout.`,
	RunE: runWait,
}

var (
	flagWaitInterval time.Duration
	flagWaitTimeout  time.Duration
	flagWaitGrace    time.Duration
)

func init() {
	waitCmd.Flags().DurationVar(&flagWaitInterval, "interval", 2*time.Second, "Poll interval")
	waitCmd.Flags().DurationVar(&flagWaitTimeout, "timeout", 0, "Give up after this long (0 = wait forever)")
	waitCmd.Flags().DurationVar(&flagWaitGrace, "grace", 15*time.Second, "Keep polling an agent-less snapshot this long (covers agent startup)")
	rootCmd.AddCommand(waitCmd)

	_ = waitCmd.RegisterFlagCompletionFunc("interval", cobra.NoFileCompletions)
}

func runWait(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	root, state := classifyCwd(ctx)
	if state == cwdNotRepo {
		return fmt.Errorf("not in a git repository")
	}

	result, err := agentcmd.NewHandler(newAgentDeps()).WaitSettled(ctx, root, args, agentcmd.WaitOptions{
		Interval: flagWaitInterval,
		Timeout:  flagWaitTimeout,
		Grace:    flagWaitGrace,
	})
	if printErr := cli.PrintJSON(result); printErr != nil {
		return printErr
	}
	return err
}
