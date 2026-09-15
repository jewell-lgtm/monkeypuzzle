package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellInitCmd = &cobra.Command{
	Use:       "shell-init [bash|zsh|fish]",
	Short:     "Print a shell wrapper that follows mp into the worktree",
	ValidArgs: []string{"bash", "zsh", "fish"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Long: `Print a shell function that makes mp change your directory.

mp itself never touches your shell: with no multiplexer session to attach, it
prints the worktree path and leaves you where you are. This wrapper closes that
loop — it runs mp with ` + "`" + EnvCwdFile + "`" + ` set, and cd's to whatever mp records
there. Nothing changes for any other caller, and mp's stdout is untouched, so
pipes and ` + "`cd \"$(mp switch x)\"`" + ` keep working.

Verbs that move you: switch, go, create, adopt, inbox next/prev, agent focus,
and done/abandon/cleanup when they remove the worktree you're standing in.

Add to your shell config:

  eval "$(mp shell-init zsh)"       # ~/.zshrc
  eval "$(mp shell-init bash)"      # ~/.bashrc
  mp shell-init fish | source       # ~/.config/fish/config.fish

Tab completion is separate — see ` + "`mp completion`" + `.`,
	RunE: runShellInit,
}

func init() {
	rootCmd.AddCommand(shellInitCmd)
}

func runShellInit(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "bash", "zsh":
		fmt.Printf(posixShellInit, EnvCwdFile)
	case "fish":
		fmt.Printf(fishShellInit, EnvCwdFile)
	}
	return nil
}

// posixShellInit is the bash/zsh wrapper. It preserves mp's exit code, leaves
// stdout alone, and falls back to a plain call if the temp file can't be made.
const posixShellInit = `# monkeypuzzle shell integration — eval "$(mp shell-init zsh)"
export MP_SHELL_INIT=1
mp() {
  local _mp_cwd_file _mp_status _mp_dir
  _mp_cwd_file="$(mktemp -t mp-cwd.XXXXXX 2>/dev/null)" || {
    command mp "$@"
    return $?
  }
  %[1]s="$_mp_cwd_file" command mp "$@"
  _mp_status=$?
  _mp_dir="$(cat "$_mp_cwd_file" 2>/dev/null)"
  rm -f -- "$_mp_cwd_file"
  if [ -n "$_mp_dir" ] && [ -d "$_mp_dir" ] && [ "$_mp_dir" != "$PWD" ]; then
    cd -- "$_mp_dir" || return $?
  fi
  return $_mp_status
}
`

// fishShellInit is the same wrapper for fish.
const fishShellInit = `# monkeypuzzle shell integration — mp shell-init fish | source
set -gx MP_SHELL_INIT 1
function mp --description 'monkeypuzzle, following it into the worktree'
    set -l cwd_file (mktemp -t mp-cwd.XXXXXX 2>/dev/null)
    if test -z "$cwd_file"
        command mp $argv
        return $status
    end
    %[1]s=$cwd_file command mp $argv
    set -l mp_status $status
    set -l dir (cat $cwd_file 2>/dev/null)
    rm -f -- $cwd_file
    if test -n "$dir" -a -d "$dir" -a "$dir" != "$PWD"
        cd -- $dir
    end
    return $mp_status
end
`
