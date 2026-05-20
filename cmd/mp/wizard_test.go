package mp

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandSkipsConfigCheck(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "mp"}
	configParent := &cobra.Command{Use: "config"}
	configSet := &cobra.Command{Use: "set"}
	pieceParent := &cobra.Command{Use: "piece"}
	pieceCreate := &cobra.Command{Use: "create"}
	pieceCreate.Flags().Bool("schema", false, "")
	completion := &cobra.Command{Use: "completion"}
	helpCmd := &cobra.Command{Use: "help"}

	configParent.AddCommand(configSet)
	pieceParent.AddCommand(pieceCreate)
	root.AddCommand(configParent, pieceParent, completion, helpCmd)

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
		want bool
	}{
		{"nil command", nil, nil, true},
		{"top-level config", configParent, nil, true},
		{"config subcommand", configSet, nil, true},
		{"completion", completion, nil, true},
		{"help", helpCmd, nil, true},
		{"piece create without schema", pieceCreate, nil, false},
		{"piece create with --schema", pieceCreate, []string{"--schema"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cmd != nil && tc.args != nil {
				// Reset flag state before parsing.
				_ = tc.cmd.Flags().Set("schema", "false")
				if err := tc.cmd.ParseFlags(tc.args); err != nil {
					t.Fatalf("parse flags: %v", err)
				}
			}
			got := commandSkipsConfigCheck(tc.cmd)
			if got != tc.want {
				t.Errorf("commandSkipsConfigCheck = %v, want %v", got, tc.want)
			}
		})
	}
}
