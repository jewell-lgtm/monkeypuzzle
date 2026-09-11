package main

import (
	"fmt"
	"os"

	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// EnvCwdFile names a file the `mp shell-init` wrapper creates per invocation.
// When set, every command that ends with "you should now be in <dir>" — a
// switch, a create, `done` leaving a removed worktree — writes that directory
// there, and the wrapper cd's into it after mp exits. Unset (the default) mp
// assumes nothing about the caller's shell and only prints the path.
const EnvCwdFile = "MP_CWD_FILE"

// noteCwd records dir for the shell wrapper, if one asked. Silent otherwise.
func noteCwd(dir string) {
	if dir == "" {
		return
	}
	if f := os.Getenv(EnvCwdFile); f != "" {
		_ = os.WriteFile(f, []byte(dir+"\n"), 0o600)
	}
}

// surfacePath is the no-session hand-off: the directory on stdout (the whole
// of stdout, so `cd "$(mp switch x)"` works) and noted for the wrapper.
func surfacePath(dir string) {
	noteCwd(dir)
	fmt.Println(dir)
}

// surfaceRoot is surfacePath for verbs that removed the worktree the caller
// stood in: on a terminal the main repo root goes on stdout with a hint, so
// the shell is not left in a deleted directory; off a terminal the JSON
// result already carries main_path, so only the wrapper is told.
func surfaceRoot(wd, removedWorktree, root string, jsonMode bool) {
	if root == "" || !piececmd.IsPathInside(wd, removedWorktree) {
		return
	}
	noteCwd(root)
	if jsonMode || !cli.IsInteractive() {
		return
	}
	fmt.Println(root)
	cli.Hint(fmt.Sprintf("cd %s — this worktree is gone", root))
}
