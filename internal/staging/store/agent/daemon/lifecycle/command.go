// Package lifecycle provides declarative agent lifecycle management for staging commands.
// It classifies commands into Write, Read, and File categories, each with different
// agent lifecycle requirements.
package lifecycle

// WriteCommand represents commands that require the agent to be running.
// The agent will be auto-started if not already running.
type WriteCommand int

// Write commands that auto-start the agent.
const (
	CmdAdd WriteCommand = iota
	CmdEdit
	CmdDelete
	CmdTag
	CmdUntag
	CmdResetVersion
	CmdStashPop
	CmdAgentStart
)

// ReadCommand represents commands that check if the agent is running.
// If the agent is not running, NothingStaged is returned instead of starting the agent.
type ReadCommand int

// Read commands that ping the agent first.
const (
	CmdStatus ReadCommand = iota
	CmdDiff
	CmdApply
	CmdResetAll
	CmdReset
	CmdStashPush
	CmdAgentStop
)

// FileCommand represents commands that only interact with file storage.
// These commands do not require the agent at all.
type FileCommand int

// File-only commands that don't interact with the agent.
const (
	CmdStashShow FileCommand = iota
	CmdStashDrop
)
