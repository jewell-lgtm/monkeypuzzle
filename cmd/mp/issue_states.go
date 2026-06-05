package mp

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/workflow"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagStatesProvider string
	flagStatesSchema   bool
)

var issueStatesCmd = &cobra.Command{
	Use:   "states",
	Short: "Dump the project's issue provider's workflow states",
	Long: `Read-only helper for populating workflow.provider_map. Currently supports
Plane only. Output is JSON to stdout.`,
	RunE: runIssueStates,
}

func init() {
	issueStatesCmd.Flags().StringVar(&flagStatesProvider, "provider", "", "Provider to query (currently: plane)")
	issueStatesCmd.Flags().BoolVar(&flagStatesSchema, "schema", false, "Output JSON schema and exit")
	issueCmd.AddCommand(issueStatesCmd)
}

func runIssueStates(cmd *cobra.Command, args []string) error {
	if flagStatesSchema {
		return cli.PrintJSON(map[string]string{"provider": ""})
	}
	if flagStatesProvider == "" {
		return fmt.Errorf("--provider is required (e.g. plane)")
	}
	if flagStatesProvider != "plane" {
		return fmt.Errorf("provider %q is not supported by `mp issue states` yet (only plane)", flagStatesProvider)
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	deps := newCLIDeps()
	cfg, err := piececmd.ReadConfig(wd, deps.FS)
	if err != nil {
		return err
	}
	if cfg.Issues.Provider != "plane" {
		return fmt.Errorf("project's issue provider is %q, not plane", cfg.Issues.Provider)
	}
	wf, err := workflow.LoadForRepo(wd, deps.FS)
	if err != nil {
		return err
	}

	prov, err := issue.NewProvider(issue.ProviderConfig{
		ProviderType: "plane",
		Config:       cfg.Issues.Config,
		Deps:         issue.ProviderDeps{FS: deps.FS, HTTP: deps.HTTP},
		Workflow:     wf,
	})
	if err != nil {
		return err
	}
	plane, ok := prov.(*issue.PlaneProvider)
	if !ok {
		return fmt.Errorf("provider is not a *PlaneProvider")
	}
	states, err := plane.ListStates()
	if err != nil {
		return err
	}
	return cli.PrintJSON(states)
}
