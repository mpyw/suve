package commands

import (
	"os"
	"slices"
)

// completionFlag is the hidden flag urfave/cli appends as the LAST argument on
// every shell TAB to request completion output instead of running the command.
const completionFlag = "--generate-shell-completion"

// IsShellCompletion reports whether args contain the hidden
// --generate-shell-completion flag that urfave/cli passes on every shell TAB.
func IsShellCompletion(args []string) bool {
	return slices.Contains(args, completionFlag)
}

// isShellCompletionInvocation reports whether the current process invocation is
// a shell-completion run, by inspecting os.Args. The --tui / --gui launch
// wrappers consult it so they fall through to normal completion instead of
// launching: urfave/cli runs Before hooks before the completion handler, so an
// unguarded wrapper would take over the (non-TTY) completion pipe and abort
// completion (#749). os.Args is the same source urfave/cli reads for the flag,
// and it mirrors main's IsShellCompletion guard on the update-check notice.
func isShellCompletionInvocation() bool {
	return IsShellCompletion(os.Args)
}
