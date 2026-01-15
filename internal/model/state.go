package model

// InputState represents the current input/interaction mode of the TUI.
// Using an enum instead of multiple boolean flags provides:
// - Mutually exclusive states (can't be in two modes at once)
// - Cleaner state transitions
// - Better pattern matching in Update()
type InputState int

const (
	// StateNormal is the default state - normal navigation and interaction
	StateNormal InputState = iota

	// StatePromptRefining is active when refining a prompt with Claude CLI
	StatePromptRefining

	// StatePlanInput is active when entering plan generation input
	StatePlanInput

	// StateContextEdit is active when editing context values (k8s, aws, etc)
	StateContextEdit

	// StateLeaderActive is active when the leader key popup is showing
	StateLeaderActive

	// StateChatOpen is active when the chat panel is open for input
	StateChatOpen

	// StateVersionView is active when viewing prompt versions
	StateVersionView
)

// String returns a human-readable name for the input state
func (s InputState) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StatePromptRefining:
		return "refining"
	case StatePlanInput:
		return "plan-input"
	case StateContextEdit:
		return "context-edit"
	case StateLeaderActive:
		return "leader"
	case StateChatOpen:
		return "chat"
	case StateVersionView:
		return "version-view"
	default:
		return "unknown"
	}
}

// IsInputBlocking returns true if the state blocks normal key handling
// (i.e., the user is typing or in a modal state)
func (s InputState) IsInputBlocking() bool {
	switch s {
	case StatePromptRefining, StatePlanInput, StateContextEdit, StateChatOpen:
		return true
	default:
		return false
	}
}

// IsModal returns true if the state shows a modal/popup overlay
func (s InputState) IsModal() bool {
	switch s {
	case StateLeaderActive, StateVersionView:
		return true
	default:
		return false
	}
}
