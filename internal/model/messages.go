package model

import "time"

// SocketMsg is sent when data is received from the socket
type SocketMsg struct {
	Payload []byte
}

// promptEditedMsg is sent when nvim finishes editing a prompt
type promptEditedMsg struct {
	path string
}

// planGeneratingMsg is sent when plan generation starts
type planGeneratingMsg struct{}

// planGeneratedMsg is sent when plan generation completes
type planGeneratedMsg struct {
	path string
	slug string
}

// planGenerateErrorMsg is sent when plan generation fails
type planGenerateErrorMsg struct {
	err error
}

// planEditedMsg is sent when plan editing completes
type planEditedMsg struct{}

// leaderTimeoutMsg is sent when leader mode should auto-dismiss
type leaderTimeoutMsg struct {
	activatedAt time.Time // To verify we're timing out the right activation
}

// ralphRefreshTickMsg is sent to trigger Ralph state refresh
type ralphRefreshTickMsg struct {
	time.Time
}

// toastTickMsg is sent to trigger toast expiration checks
type toastTickMsg struct{}

// toastCleanupTickMsg is sent to trigger periodic toast cleanup
type toastCleanupTickMsg struct {
	Time time.Time
}

// contextLoadedMsg is sent when context is loaded asynchronously
type contextLoadedMsg struct{}

// daemonHistoryMsg is sent when daemon query returns recent edits
type daemonHistoryMsg struct {
	changes []Change
	err     error
}

// daemonStatusMsg is sent when daemon status check completes
type daemonStatusMsg struct {
	connected       bool
	uptime          string
	workspaceActive bool
	workspaceEdits  int
	lastActivity    time.Time
}

// daemonStatusTickMsg is sent to trigger periodic daemon status checks
type daemonStatusTickMsg struct {
	time.Time
}
