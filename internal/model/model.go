package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/ztaylor/claude-follow-tui/internal/chat"
	"github.com/ztaylor/claude-follow-tui/internal/diff"
	"github.com/ztaylor/claude-follow-tui/internal/highlight"
	"github.com/ztaylor/claude-follow-tui/internal/history"
	"github.com/ztaylor/claude-follow-tui/internal/logger"
	"github.com/ztaylor/claude-follow-tui/internal/minimap"
	"github.com/ztaylor/claude-follow-tui/internal/plan"
	"github.com/ztaylor/claude-follow-tui/internal/prompt"
	"github.com/ztaylor/claude-follow-tui/internal/ralph"
	"github.com/ztaylor/claude-follow-tui/internal/theme"
)

// SocketMsg is sent when data is received from the socket
type SocketMsg struct {
	Payload []byte
}

// promptEditedMsg is sent when nvim finishes editing a prompt
type promptEditedMsg struct {
	path string
}

// promptRefinedMsg is sent when Claude CLI finishes refining a prompt
type promptRefinedMsg struct {
	originalPath string
	refinedPath  string
}

// promptRefineErrorMsg is sent when refining fails
type promptRefineErrorMsg struct {
	err error
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

// Change represents a single file change from Claude
type Change struct {
	Timestamp   time.Time
	FilePath    string
	ToolName    string
	OldString   string
	NewString   string
	FileContent string // Full file content after the change
	LineNum     int    // Line number where change starts
	LineCount   int    // Number of lines changed
	CommitSHA   string // VCS commit SHA at time of change
	CommitShort string // Short SHA for display
	VCSType     string // "git" or "jj"
}

// HookPayload matches the JSON structure from the Claude hook
type HookPayload struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Content   string `json:"content"`
	} `json:"tool_input"`
	Parameters struct {
		FilePath  string `json:"file_path"`
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"parameters"`
}

// Pane represents which pane is active
type Pane int

const (
	PaneLeft Pane = iota
	PaneRight
)

// ToastType represents the style of toast notification
type ToastType int

const (
	ToastInfo ToastType = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// Toast represents a notification message
type Toast struct {
	Message   string
	Type      ToastType
	CreatedAt time.Time
	Duration  time.Duration
}

// LeftPaneMode represents what the left pane is showing
type LeftPaneMode int

const (
	LeftPaneModeHistory LeftPaneMode = iota
	LeftPaneModePrompts
	LeftPaneModeRalph
	LeftPaneModePlan
)

// Model is the Bubbletea model
type Model struct {
	socketPath     string
	width          int
	height         int
	activePane     Pane
	leftPaneMode   LeftPaneMode // History or Prompts mode
	changes        []Change
	selectedIndex  int
	diffViewport   viewport.Model
	showHelp       bool
	showMinimap    bool // Toggle minimap visibility
	planContent    string
	planPath       string
	planViewport   viewport.Model
	ready          bool
	theme          *theme.Theme
	highlighter    *highlight.Highlighter
	scrollX        int              // Horizontal scroll offset
	totalLines     int              // Total lines in current file (for minimap)
	minimapData    *minimap.Minimap // Cached minimap line types
	diffCache      map[int]string   // Cached rendered diffs by index
	historyStore   *history.Store   // Persistent history storage
	persistHistory bool             // Whether to save history to file

	// Prompt manager (integrated in left pane)
	promptStore        *prompt.Store          // Prompt storage
	promptList         []prompt.Prompt        // Cached list of prompts
	promptSelected     int                    // Selected prompt index
	promptRefining     bool                   // Whether we're in refine mode
	promptInjectMethod prompt.InjectionMethod // Current injection method

	// Version view mode
	promptShowVersions    bool                   // Whether showing version list
	promptVersions        []prompt.PromptVersion // List of versions for selected prompt
	promptVersionSelected int                    // Selected version index

	// Toast notifications
	toasts []Toast // Active toast notifications

	// Ralph mode state
	ralphState *ralph.State

	// Plan generation
	planInputActive bool            // Whether plan input is active
	planInput       textinput.Model // Plan description input
	planGenerating  bool            // Whether plan is being generated

	// Claude chat
	chatActive   bool             // Whether chat panel is active
	chat         *chat.ClaudeChat // Chat subprocess manager
	chatInput    textinput.Model  // Chat input field
	chatViewport viewport.Model   // Chat output viewport
}

// Option is a functional option for configuring the Model
type Option func(*Model)

// WithTheme sets the theme for the model
func WithTheme(t *theme.Theme) Option {
	return func(m *Model) {
		m.theme = t
	}
}

// WithPersistence enables file-based history persistence
func WithPersistence(enabled bool) Option {
	return func(m *Model) {
		m.persistHistory = enabled
	}
}

// New creates a new Model with optional configuration
func New(socketPath string, opts ...Option) Model {
	t := theme.Default()
	m := Model{
		socketPath:   socketPath,
		changes:      []Change{},
		activePane:   PaneLeft,
		leftPaneMode: LeftPaneModeHistory,
		showMinimap:  true,
		theme:        t,
		highlighter:  highlight.NewHighlighter(t),
		diffCache:    make(map[int]string),
	}

	for _, opt := range opts {
		opt(&m)
	}

	// Recreate highlighter if theme was changed via option
	if m.highlighter == nil || m.highlighter.Theme() != m.theme {
		m.highlighter = highlight.NewHighlighter(m.theme)
	}

	// Initialize prompt store
	if store, err := prompt.NewStore(); err == nil {
		m.promptStore = store
		m.promptInjectMethod = prompt.DetectBestMethod()
	} else {
		logger.Log("Failed to initialize prompt store: %v", err)
	}

	// Initialize history store if persistence is enabled
	if m.persistHistory {
		m.historyStore = history.NewStore(history.GetHistoryPath())
		if err := m.historyStore.Load(); err != nil {
			logger.Log("Failed to load history: %v", err)
		} else {
			// Convert history entries to changes
			for _, entry := range m.historyStore.Entries() {
				m.changes = append(m.changes, Change{
					Timestamp:   entry.Timestamp,
					FilePath:    entry.FilePath,
					ToolName:    entry.ToolName,
					OldString:   entry.OldString,
					NewString:   entry.NewString,
					LineNum:     entry.LineNum,
					LineCount:   entry.LineCount,
					CommitSHA:   entry.CommitSHA,
					CommitShort: entry.CommitShort,
					VCSType:     entry.VCSType,
				})
			}
			logger.Log("Loaded %d history entries", len(m.changes))
		}
	}

	// Load active plan file on startup
	m.loadPlanFile()
	if m.planPath != "" {
		logger.Log("Loaded plan file: %s", m.planPath)
	}

	// Initialize plan input
	ti := textinput.New()
	ti.Placeholder = "Describe what you want to build..."
	ti.CharLimit = 500
	ti.Width = 60
	m.planInput = ti

	// Initialize chat input
	chatTi := textinput.New()
	chatTi.Placeholder = "Type a message..."
	chatTi.CharLimit = 1000
	chatTi.Width = 60
	m.chatInput = chatTi

	return m
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// addToast adds a new toast notification
func (m *Model) addToast(message string, toastType ToastType) {
	m.toasts = append(m.toasts, Toast{
		Message:   message,
		Type:      toastType,
		CreatedAt: time.Now(),
		Duration:  3 * time.Second,
	})
	// Limit to 5 toasts max
	if len(m.toasts) > 5 {
		m.toasts = m.toasts[len(m.toasts)-5:]
	}
}

// cleanExpiredToasts removes toasts that have exceeded their duration
func (m *Model) cleanExpiredToasts() {
	now := time.Now()
	active := make([]Toast, 0, len(m.toasts))
	for _, t := range m.toasts {
		if now.Sub(t.CreatedAt) < t.Duration {
			active = append(active, t)
		}
	}
	m.toasts = active
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Clean expired toasts on any update
	m.cleanExpiredToasts()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Initialize/resize viewport for diff
		headerHeight := 3
		footerHeight := 2
		if m.diffViewport.Width == 0 {
			m.diffViewport = viewport.New(m.width/2-4, m.height-headerHeight-footerHeight-2)
		}
		m.updateViewportSize()
		m.diffViewport.SetContent(m.renderDiff())

	case tea.MouseMsg:
		// Handle mouse scroll in diff pane
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.diffViewport.LineUp(3)
			case tea.MouseButtonWheelDown:
				m.diffViewport.LineDown(3)
			}
		}

	case tea.KeyMsg:
		logger.Log("KeyMsg received: %q", msg.String())
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Global keys (work in any mode)
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "tab":
			// Cycle to next tab/mode
			m.cycleMode(1)
			return m, nil
		case "shift+tab":
			// Cycle to previous tab/mode
			m.cycleMode(-1)
			return m, nil
		case "[":
			// Switch to left pane
			m.activePane = PaneLeft
			return m, nil
		case "]":
			// Switch to right pane
			m.activePane = PaneRight
			return m, nil
		case "1":
			// Direct access to History tab
			m.switchToMode(LeftPaneModeHistory)
			return m, nil
		case "2":
			// Direct access to Prompts tab
			m.switchToMode(LeftPaneModePrompts)
			return m, nil
		case "3":
			// Direct access to Ralph tab
			m.switchToMode(LeftPaneModeRalph)
			return m, nil
		case "4":
			// Direct access to Plan tab
			m.switchToMode(LeftPaneModePlan)
			return m, nil
		case "m":
			m.showMinimap = !m.showMinimap
			m.updateViewportSize()
			m.diffViewport.SetContent(m.renderRightPane())
			return m, nil
		case "c":
			// Toggle chat panel (only if not in chat input mode)
			if !m.chatActive {
				return m.startChat()
			}
		}

		// Handle chat mode keys
		if m.chatActive {
			return m.handleChatKeys(msg)
		}

		// Mode-specific key handling
		switch m.leftPaneMode {
		case LeftPaneModePrompts:
			return m.handlePromptsKeys(msg)
		case LeftPaneModeRalph:
			return m.handleRalphKeys(msg)
		case LeftPaneModePlan:
			return m.handlePlanKeys(msg)
		default:
			return m.handleHistoryKeys(msg)
		}

	case SocketMsg:
		logger.Log("SocketMsg received, payload size: %d bytes", len(msg.Payload))

		// Extract plan_path from payload if present (sent by hook)
		var planInfo struct {
			PlanPath string `json:"plan_path"`
		}
		if json.Unmarshal(msg.Payload, &planInfo) == nil && planInfo.PlanPath != "" {
			m.planPath = planInfo.PlanPath
			logger.Log("Received planPath from hook: %s", m.planPath)
		}

		change := parsePayload(msg.Payload)
		if change != nil {
			// Get current VCS commit info
			sha, shortSHA, vcsType := history.GetCurrentCommit()
			change.CommitSHA = sha
			change.CommitShort = shortSHA
			change.VCSType = vcsType

			logger.Log("Parsed change: %s %s (line %d) commit=%s fileContent=%d bytes", change.ToolName, change.FilePath, change.LineNum, shortSHA, len(change.FileContent))
			// Append new change to end of list (queue style)
			m.changes = append(m.changes, *change)
			logger.Log("Total changes now: %d, selectedIndex: %d", len(m.changes), m.selectedIndex)

			// Save to history if persistence enabled
			if m.persistHistory && m.historyStore != nil {
				entry := history.Entry{
					Timestamp:   change.Timestamp,
					FilePath:    change.FilePath,
					ToolName:    change.ToolName,
					OldString:   change.OldString,
					NewString:   change.NewString,
					LineNum:     change.LineNum,
					LineCount:   change.LineCount,
					CommitSHA:   change.CommitSHA,
					CommitShort: change.CommitShort,
					VCSType:     change.VCSType,
				}
				if err := m.historyStore.Add(entry); err != nil {
					logger.Log("Failed to save history: %v", err)
				}
			}

			// If this is the first change, select it
			if len(m.changes) == 1 {
				m.selectedIndex = 0
				m.diffViewport.SetContent(m.renderDiff())
			}
			// Otherwise keep current position - user can press 'n' to see new items
		} else {
			logger.Log("parsePayload returned nil")
		}

	case promptEditedMsg:
		// Prompt was edited in nvim - refresh list
		logger.Log("Prompt edited: %s, leftPaneMode=%d", msg.path, m.leftPaneMode)
		m.promptRefining = false
		m.leftPaneMode = LeftPaneModePrompts // Ensure we stay in prompts mode
		m.refreshPromptList()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Prompt saved", ToastSuccess)

	case promptRefinedMsg:
		// Claude refined the prompt - open nvim diff
		logger.Log("Prompt refined: %s vs %s", msg.originalPath, msg.refinedPath)
		cmd := exec.Command("nvim", "-d", msg.originalPath, msg.refinedPath)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			// Clean up temp file
			os.Remove(msg.refinedPath)
			return promptEditedMsg{path: msg.originalPath}
		})

	case promptRefineErrorMsg:
		logger.Log("Prompt refine error: %v", msg.err)
		m.promptRefining = false
		m.addToast("Refine failed: "+msg.err.Error(), ToastError)

	case planGeneratedMsg:
		logger.Log("Plan generated: %s", msg.path)
		m.planGenerating = false
		m.planPath = msg.path
		m.loadPlanFile()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Plan created: "+msg.slug, ToastSuccess)

	case planGenerateErrorMsg:
		logger.Log("Plan generate error: %v", msg.err)
		m.planGenerating = false
		m.addToast("Plan generation failed: "+msg.err.Error(), ToastError)
	}

	return m, tea.Batch(cmds...)
}

// handleHistoryKeys handles key events in history mode
func (m Model) handleHistoryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.activePane == PaneLeft {
			// Navigate history
			if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
				m.selectedIndex++
				m.scrollX = 0
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.diffViewport.LineDown(1)
		}
	case "k", "up":
		if m.activePane == PaneLeft {
			// Navigate history
			if len(m.changes) > 0 && m.selectedIndex > 0 {
				m.selectedIndex--
				m.scrollX = 0
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.diffViewport.LineUp(1)
		}
	case "n":
		// Next change in queue
		if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
			m.selectedIndex++
			m.scrollX = 0
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}
	case "p":
		// Previous change in queue
		if len(m.changes) > 0 && m.selectedIndex > 0 {
			m.selectedIndex--
			m.scrollX = 0
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}
	case "left":
		if m.scrollX > 0 {
			m.scrollX -= 4
			if m.scrollX < 0 {
				m.scrollX = 0
			}
			m.diffViewport.SetContent(m.renderDiff())
		}
	case "right":
		m.scrollX += 4
		m.diffViewport.SetContent(m.renderDiff())
	case "c":
		m.changes = []Change{}
		m.selectedIndex = 0
		m.diffViewport.SetContent("")
		m.diffCache = make(map[int]string)
		if m.persistHistory && m.historyStore != nil {
			if err := m.historyStore.Clear(); err != nil {
				logger.Log("Failed to clear history file: %v", err)
			}
		}
	case "ctrl+g":
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	case "ctrl+o":
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	}
	return m, nil
}

// handlePromptsKeys handles key events in prompts mode
func (m Model) handlePromptsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Version view mode has different key bindings
	if m.promptShowVersions {
		switch msg.String() {
		case "V", "shift+v", "esc":
			// Exit version view, back to prompt list
			m.promptShowVersions = false
			m.promptVersionSelected = 0
			m.diffViewport.SetContent(m.renderRightPane())
		case "j", "down":
			if m.promptVersionSelected < len(m.promptVersions)-1 {
				m.promptVersionSelected++
				m.diffViewport.SetContent(m.renderRightPane())
			}
		case "k", "up":
			if m.promptVersionSelected > 0 {
				m.promptVersionSelected--
				m.diffViewport.SetContent(m.renderRightPane())
			}
		case "r", "enter":
			// Revert to selected version
			if len(m.promptVersions) > 0 && len(m.promptList) > 0 && m.promptStore != nil {
				v := m.promptVersions[m.promptVersionSelected]
				p := m.promptList[m.promptSelected]
				if err := m.promptStore.RestoreVersion(p.Path, v.Version); err != nil {
					m.addToast(err.Error(), ToastError)
				} else {
					m.addToast(fmt.Sprintf("Reverted to v%d", v.Version), ToastSuccess)
					m.refreshPromptList()
					m.promptShowVersions = false
					m.diffViewport.SetContent(m.renderRightPane())
				}
			}
		case "ctrl+d":
			// Delete version file
			if len(m.promptVersions) > 0 {
				v := m.promptVersions[m.promptVersionSelected]
				if err := os.Remove(v.Path); err != nil {
					m.addToast(err.Error(), ToastError)
				} else {
					m.addToast(fmt.Sprintf("Deleted v%d", v.Version), ToastSuccess)
					m.loadVersionList()
					if m.promptVersionSelected >= len(m.promptVersions) && m.promptVersionSelected > 0 {
						m.promptVersionSelected--
					}
					if len(m.promptVersions) == 0 {
						m.promptShowVersions = false
					}
					m.diffViewport.SetContent(m.renderRightPane())
				}
			}
		case "e":
			// Open version in editor (read-only view)
			if len(m.promptVersions) > 0 {
				v := m.promptVersions[m.promptVersionSelected]
				cmd := exec.Command("nvim", "-R", v.Path)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return promptEditedMsg{path: v.Path}
				})
			}
		}
		return m, nil
	}

	// Normal prompt list mode
	switch msg.String() {
	case "j", "down":
		if m.activePane == PaneLeft && m.promptSelected < len(m.promptList)-1 {
			m.promptSelected++
			m.diffViewport.SetContent(m.renderRightPane())
		} else if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case "k", "up":
		if m.activePane == PaneLeft && m.promptSelected > 0 {
			m.promptSelected--
			m.diffViewport.SetContent(m.renderRightPane())
		} else if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case "n":
		// New project-local prompt - open nvim with template
		return m.createNewPrompt(false)
	case "N":
		// New global prompt - open nvim with template
		return m.createNewPrompt(true)
	case "e":
		// Edit selected prompt
		if len(m.promptList) > 0 {
			return m.editPrompt(m.promptList[m.promptSelected])
		}
	case "r":
		// Refine prompt with Claude
		if len(m.promptList) > 0 && !m.promptRefining {
			m.addToast("Refining with Claude...", ToastInfo)
			return m.refinePrompt(m.promptList[m.promptSelected])
		}
	case "v":
		// Create version backup
		logger.Log("Version key pressed: promptList=%d, promptStore=%v", len(m.promptList), m.promptStore != nil)
		if len(m.promptList) > 0 && m.promptStore != nil {
			p := m.promptList[m.promptSelected]
			logger.Log("Creating version for: %s (path=%s)", p.Name, p.Path)
			if err := m.promptStore.CreateVersion(&p); err != nil {
				logger.Log("CreateVersion error: %v", err)
				m.addToast(err.Error(), ToastError)
			} else {
				logger.Log("CreateVersion success: v%d", p.Version)
				m.addToast(fmt.Sprintf("Created v%d backup", p.Version), ToastSuccess)
				m.refreshPromptList()
				m.diffViewport.SetContent(m.renderRightPane())
			}
		} else {
			logger.Log("Version skipped: no prompts or no store")
		}
	case "V", "shift+v":
		// Enter version view mode
		if len(m.promptList) > 0 && m.promptStore != nil {
			m.loadVersionList()
			if len(m.promptVersions) > 0 {
				m.promptShowVersions = true
				m.promptVersionSelected = 0
				m.diffViewport.SetContent(m.renderRightPane())
			} else {
				m.addToast("No versions found", ToastWarning)
			}
		}
	case "ctrl+d":
		// Delete prompt
		if len(m.promptList) > 0 && m.promptStore != nil {
			p := m.promptList[m.promptSelected]
			if err := m.promptStore.Delete(p.Path); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast("Deleted "+p.Name, ToastSuccess)
				m.refreshPromptList()
				if m.promptSelected >= len(m.promptList) && m.promptSelected > 0 {
					m.promptSelected--
				}
				m.diffViewport.SetContent(m.renderRightPane())
			}
		}
	case "enter":
		// Inject prompt using current method
		if len(m.promptList) > 0 {
			p := m.promptList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			logger.Log("Injecting prompt: original=%d bytes, expanded=%d bytes", len(p.Content), len(expanded))
			if err := prompt.Inject(expanded, m.promptInjectMethod); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast(fmt.Sprintf("Sent via %s", prompt.MethodName(m.promptInjectMethod)), ToastSuccess)
			}
		}
	case "y":
		// Yank/copy to clipboard only
		if len(m.promptList) > 0 {
			p := m.promptList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			if err := prompt.Inject(expanded, prompt.InjectClipboard); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast("Copied to clipboard", ToastSuccess)
			}
		}
	case "i":
		// Cycle injection method
		m.promptInjectMethod = (m.promptInjectMethod + 1) % 3
		m.addToast(fmt.Sprintf("Inject method: %s", prompt.MethodName(m.promptInjectMethod)), ToastInfo)
	}
	return m, nil
}

// handleRalphKeys handles key events in Ralph mode
func (m Model) handleRalphKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case "k", "up":
		if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case "C":
		// Cancel Ralph loop
		if m.ralphState != nil && m.ralphState.Active {
			if removed, _ := ralph.CancelLoop(); removed {
				m.ralphState = nil
				m.addToast("Ralph Loop cancelled", ToastSuccess)
				m.diffViewport.SetContent(m.renderRightPane())
			}
		}
	case "r":
		// Refresh Ralph state
		m.loadRalphState()
		m.diffViewport.SetContent(m.renderRightPane())
	}
	return m, nil
}

// handlePlanKeys handles key events in Plan mode
func (m Model) handlePlanKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle plan input mode
	if m.planInputActive {
		switch msg.String() {
		case "enter":
			// Submit plan description
			description := m.planInput.Value()
			if description != "" {
				m.planInputActive = false
				m.planGenerating = true
				m.planInput.Reset()
				m.addToast("Generating plan...", ToastInfo)
				return m, m.generatePlan(description)
			}
		case "esc":
			// Cancel plan input
			m.planInputActive = false
			m.planInput.Reset()
			return m, nil
		default:
			// Forward to textinput
			var cmd tea.Cmd
			m.planInput, cmd = m.planInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case "k", "up":
		if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case "d":
		if m.activePane == PaneRight {
			m.diffViewport.HalfViewDown()
		}
	case "u":
		if m.activePane == PaneRight {
			m.diffViewport.HalfViewUp()
		}
	case "G":
		// Generate new plan
		if !m.planGenerating {
			m.planInputActive = true
			m.planInput.Focus()
			return m, textinput.Blink
		}
	case "e":
		// Edit plan in nvim
		if m.planPath != "" {
			cmd := exec.Command("nvim", m.planPath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	case "r":
		// Refresh plan
		m.loadPlanFile()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Plan refreshed", ToastInfo)
	}
	return m, nil
}

// generatePlan runs Claude CLI to generate a plan
func (m Model) generatePlan(description string) tea.Cmd {
	return func() tea.Msg {
		path, err := plan.Generate(description)
		if err != nil {
			return planGenerateErrorMsg{err: err}
		}
		slug := strings.TrimSuffix(filepath.Base(path), ".md")
		return planGeneratedMsg{path: path, slug: slug}
	}
}

// startChat starts a new chat session
func (m *Model) startChat() (Model, tea.Cmd) {
	m.chatActive = true
	m.chat = chat.New()

	// Write MCP config for chat
	mcpConfigPath, err := plan.WriteMCPConfig()
	if err != nil {
		m.addToast("Failed to write MCP config: "+err.Error(), ToastError)
		m.chatActive = false
		return *m, nil
	}

	// Start the chat subprocess
	if err := m.chat.Start(mcpConfigPath); err != nil {
		m.addToast("Failed to start chat: "+err.Error(), ToastError)
		m.chatActive = false
		return *m, nil
	}

	// Focus the chat input
	m.chatInput.Focus()
	m.addToast("Chat started", ToastInfo)

	// Return with blink command for the input
	return *m, textinput.Blink
}

// stopChat stops the chat session
func (m *Model) stopChat() {
	if m.chat != nil {
		m.chat.Stop()
	}
	m.chatActive = false
	m.chatInput.Blur()
	m.chatInput.Reset()
	m.addToast("Chat closed", ToastInfo)
}

// handleChatKeys handles key events when chat is active
func (m Model) handleChatKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Close chat
		m.stopChat()
		return m, nil
	case "ctrl+c":
		// Kill chat and close
		m.stopChat()
		return m, nil
	case "ctrl+l":
		// Clear chat output
		if m.chat != nil {
			m.chat.ClearOutput()
		}
		return m, nil
	case "enter":
		// Send message
		input := m.chatInput.Value()
		if input != "" && m.chat != nil {
			if err := m.chat.Send(input); err != nil {
				m.addToast("Failed to send: "+err.Error(), ToastError)
			}
			m.chatInput.Reset()
		}
		return m, nil
	default:
		// Forward to chat input
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	}
}

// renderChatPane renders the chat interface
func (m *Model) renderChatPane() string {
	var sb strings.Builder
	width := m.diffViewport.Width

	sb.WriteString(m.theme.Title.Render("Claude Chat") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", width-4)) + "\n\n")

	if m.chat == nil {
		sb.WriteString(m.theme.Dim.Render("Chat not active"))
		return sb.String()
	}

	// Show chat output
	output := m.chat.Output()
	if output == "" {
		sb.WriteString(m.theme.Dim.Render("Waiting for response...\n\n"))
	} else {
		// Render output with basic formatting
		lines := strings.Split(output, "\n")
		maxLines := m.diffViewport.Height - 10
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		for _, line := range lines {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	// Show input area
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", width-4)) + "\n")
	sb.WriteString(m.chatInput.View() + "\n\n")
	sb.WriteString(m.theme.Dim.Render("Enter:send  Esc:close  ^L:clear"))

	return sb.String()
}

// cycleMode cycles through the available modes
func (m *Model) cycleMode(direction int) {
	modes := []LeftPaneMode{LeftPaneModeHistory, LeftPaneModePrompts, LeftPaneModeRalph, LeftPaneModePlan}
	currentIdx := 0
	for i, mode := range modes {
		if mode == m.leftPaneMode {
			currentIdx = i
			break
		}
	}

	newIdx := (currentIdx + direction + len(modes)) % len(modes)
	m.switchToMode(modes[newIdx])
}

// switchToMode switches to a specific mode
func (m *Model) switchToMode(mode LeftPaneMode) {
	prevMode := m.leftPaneMode
	m.leftPaneMode = mode
	m.activePane = PaneLeft
	m.promptShowVersions = false

	// Mode-specific initialization
	switch mode {
	case LeftPaneModePrompts:
		m.refreshPromptList()
	case LeftPaneModeRalph:
		m.loadRalphState()
	case LeftPaneModePlan:
		m.loadPlanFile()
	}

	m.updateViewportSize()
	m.diffViewport.SetContent(m.renderRightPane())
	logger.Log("Switched from %d to %d mode", prevMode, mode)
}

// loadRalphState loads the Ralph Loop state from the state file
func (m *Model) loadRalphState() {
	state, err := ralph.LoadState()
	if err != nil {
		logger.Log("Failed to load Ralph state: %v", err)
		m.ralphState = nil
		return
	}
	m.ralphState = state
	if state != nil {
		logger.Log("Loaded Ralph state: active=%v, iteration=%d/%d", state.Active, state.Iteration, state.MaxIterations)
	}
}

// renderTabBar renders the tab bar with all 4 modes
func (m Model) renderTabBar() string {
	tabs := []struct {
		num  string
		name string
		mode LeftPaneMode
		icon string
	}{
		{"1", "History", LeftPaneModeHistory, ""},
		{"2", "Prompts", LeftPaneModePrompts, ""},
		{"3", "Ralph", LeftPaneModeRalph, "🔄"},
		{"4", "Plan", LeftPaneModePlan, "📋"},
	}

	var parts []string
	for _, tab := range tabs {
		label := tab.num + ":" + tab.name
		if tab.icon != "" {
			label = tab.num + ":" + tab.icon
		}

		if tab.mode == m.leftPaneMode {
			// Active tab - highlighted
			parts = append(parts, m.theme.Selected.Render("["+label+"]"))
		} else {
			// Inactive tab - show state indicator
			stateIndicator := ""
			switch tab.mode {
			case LeftPaneModeRalph:
				if m.ralphState != nil && m.ralphState.Active {
					stateIndicator = "•"
				}
			case LeftPaneModePlan:
				if m.planPath != "" {
					stateIndicator = "•"
				}
			}
			parts = append(parts, m.theme.Dim.Render(label+stateIndicator))
		}
	}

	return strings.Join(parts, " ")
}

// renderRalphStatus renders the Ralph status for the left pane
func (m Model) renderRalphStatus() string {
	var sb strings.Builder
	listWidth := m.width / 3

	sb.WriteString(m.theme.Title.Render("Ralph Loop") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	if m.ralphState == nil || !m.ralphState.Active {
		sb.WriteString(m.theme.Dim.Render("No active Ralph loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start a Ralph loop with:\n"))
		sb.WriteString(m.theme.Dim.Render("/ralph-loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Press 'r' to refresh"))
		return sb.String()
	}

	// Active Ralph loop status
	sb.WriteString(m.theme.Selected.Render("🔄 Active") + "\n\n")

	// Iteration progress
	progress := fmt.Sprintf("Iteration: %d / %d", m.ralphState.Iteration, m.ralphState.MaxIterations)
	sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

	// Completion promise
	if m.ralphState.Promise != "" {
		sb.WriteString(m.theme.Dim.Render("Promise: ") + "\n")
		promise := m.ralphState.Promise
		if len(promise) > listWidth-6 {
			promise = promise[:listWidth-9] + "..."
		}
		sb.WriteString(m.theme.Normal.Render("\""+promise+"\"") + "\n\n")
	}

	// Started at
	if !m.ralphState.StartedAt.IsZero() {
		durationStr := ralph.FormatDuration(time.Since(m.ralphState.StartedAt))
		sb.WriteString(m.theme.Dim.Render("Started: "+durationStr) + "\n\n")
	}

	sb.WriteString(m.theme.Dim.Render("Press 'C' to cancel"))

	return sb.String()
}

// renderPlanList renders the plan info for the left pane
func (m Model) renderPlanList() string {
	var sb strings.Builder
	listWidth := m.width / 3

	sb.WriteString(m.theme.Title.Render("Plan") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	// Show plan input if active
	if m.planInputActive {
		sb.WriteString(m.theme.Normal.Render("New Plan\n\n"))
		sb.WriteString(m.theme.Dim.Render("Describe what to build:\n\n"))
		sb.WriteString(m.planInput.View() + "\n\n")
		sb.WriteString(m.theme.Dim.Render("Enter:submit  Esc:cancel"))
		return sb.String()
	}

	// Show generating status
	if m.planGenerating {
		sb.WriteString(m.theme.Selected.Render("⏳ Generating...") + "\n\n")
		sb.WriteString(m.theme.Dim.Render("Claude is creating your plan.\n"))
		sb.WriteString(m.theme.Dim.Render("This may take a moment."))
		return sb.String()
	}

	if m.planPath == "" {
		sb.WriteString(m.theme.Dim.Render("No active plan\n\n"))
		sb.WriteString(m.theme.Dim.Render("Press 'G' to generate a new\n"))
		sb.WriteString(m.theme.Dim.Render("plan with Claude.\n\n"))
		sb.WriteString(m.theme.Dim.Render("Or press 'r' to refresh if\n"))
		sb.WriteString(m.theme.Dim.Render("Claude created one."))
		return sb.String()
	}

	// Show current plan info
	planName := strings.TrimSuffix(filepath.Base(m.planPath), ".md")
	sb.WriteString(m.theme.Selected.Render("📋 "+planName) + "\n\n")

	// Plan file location
	sb.WriteString(m.theme.Dim.Render("Location:") + "\n")
	location := m.planPath
	if len(location) > listWidth-6 {
		location = "..." + location[len(location)-listWidth+9:]
	}
	sb.WriteString(m.theme.Normal.Render(location) + "\n\n")

	// File info
	if info, err := os.Stat(m.planPath); err == nil {
		sb.WriteString(m.theme.Dim.Render("Modified: "+info.ModTime().Format("2006-01-02 15:04")) + "\n")
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("Size: %d bytes", info.Size())) + "\n\n")
	}

	sb.WriteString(m.theme.Dim.Render("G:new  e:edit  r:refresh"))

	return sb.String()
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	// Render header with tab bar
	tabBar := m.renderTabBar()

	// Queue position indicator (only in history mode)
	queueIndicator := ""
	if m.leftPaneMode == LeftPaneModeHistory && len(m.changes) > 0 {
		queueIndicator = m.theme.Selected.Render(fmt.Sprintf(" [%d/%d]", m.selectedIndex+1, len(m.changes)))
	}

	// Refining status indicator
	refineIndicator := ""
	if m.promptRefining {
		refineIndicator = m.theme.Selected.Render(" ⏳ Refining...")
	}

	// Plan indicator (show active plan file name)
	planIndicator := ""
	if m.planPath != "" {
		planName := strings.TrimSuffix(filepath.Base(m.planPath), ".md")
		planIndicator = m.theme.Dim.Render(" 📋 " + planName)
	}

	header := m.theme.Title.Render("Claude Follow") + " " + tabBar + queueIndicator + refineIndicator + planIndicator + "  " +
		m.theme.Dim.Render(time.Now().Format("15:04:05"))
	header = lipgloss.PlaceHorizontal(m.width, lipgloss.Left, header)

	// Two-pane layout
	minimapStr := m.renderMinimap()
	minimapWidth := 0
	if m.showMinimap {
		minimapWidth = 2
	}

	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 3 - minimapWidth

	// Render left pane based on current mode
	var leftContent string
	switch m.leftPaneMode {
	case LeftPaneModePrompts:
		leftContent = m.renderPromptsList()
	case LeftPaneModeRalph:
		leftContent = m.renderRalphStatus()
	case LeftPaneModePlan:
		leftContent = m.renderPlanList()
	default:
		leftContent = m.renderHistory()
	}

	leftBox := m.theme.Border
	if m.activePane == PaneLeft {
		leftBox = m.theme.ActiveBorder
	}
	leftPane := leftBox.
		Width(leftWidth).
		Height(m.height - 4).
		Render(leftContent)

	// Render right pane (diff or prompt preview)
	rightContent := m.diffViewport.View()
	rightBox := m.theme.Border
	if m.activePane == PaneRight {
		rightBox = m.theme.ActiveBorder
	}
	rightPane := rightBox.
		Width(rightWidth).
		Height(m.height - 4).
		Render(rightContent)

	var content string
	if m.showMinimap {
		content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane, minimapStr)
	} else {
		content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	}

	// Render status bar
	status := m.renderStatus()

	// Build main view
	mainView := lipgloss.JoinVertical(lipgloss.Left, header, content, status)

	// Overlay toasts in top-right corner
	if len(m.toasts) > 0 {
		toastView := m.renderToasts()
		// Calculate position for top-right
		toastWidth := lipgloss.Width(toastView)

		// Split main view into lines to overlay
		lines := strings.Split(mainView, "\n")
		toastLines := strings.Split(toastView, "\n")

		// Overlay toast on top-right (starting at line 1 to avoid header)
		for i, toastLine := range toastLines {
			lineIdx := i + 1 // Start below header
			if lineIdx < len(lines) {
				mainLine := lines[lineIdx]
				mainLineWidth := lipgloss.Width(mainLine)

				// Calculate where to place toast (right-aligned with 1 char margin)
				targetPos := m.width - toastWidth - 1
				if targetPos < 0 {
					targetPos = 0
				}

				if targetPos < mainLineWidth {
					// Truncate main line content and append toast
					// Use MaxWidth for proper ANSI-aware truncation without padding
					truncated := lipgloss.NewStyle().MaxWidth(targetPos).Render(mainLine)
					lines[lineIdx] = truncated + toastLine
				} else {
					// Pad to reach target position, then add toast
					padding := strings.Repeat(" ", targetPos-mainLineWidth)
					lines[lineIdx] = mainLine + padding + toastLine
				}
			}
		}
		mainView = strings.Join(lines, "\n")
	}

	return mainView
}

// renderToasts renders the toast notifications
func (m Model) renderToasts() string {
	if len(m.toasts) == 0 {
		return ""
	}

	var sb strings.Builder

	for i := len(m.toasts) - 1; i >= 0; i-- {
		t := m.toasts[i]

		// Style based on toast type
		var style lipgloss.Style
		var icon string
		switch t.Type {
		case ToastSuccess:
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#2d5a27")).
				Foreground(lipgloss.Color("#90EE90")).
				Padding(0, 1).
				Bold(true)
			icon = "✓ "
		case ToastError:
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#5a2727")).
				Foreground(lipgloss.Color("#ff6b6b")).
				Padding(0, 1).
				Bold(true)
			icon = "✗ "
		case ToastWarning:
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#5a4a27")).
				Foreground(lipgloss.Color("#ffd93d")).
				Padding(0, 1).
				Bold(true)
			icon = "⚠ "
		default: // ToastInfo
			style = lipgloss.NewStyle().
				Background(lipgloss.Color("#27405a")).
				Foreground(lipgloss.Color("#87CEEB")).
				Padding(0, 1).
				Bold(true)
			icon = "ℹ "
		}

		// Truncate long messages
		msg := t.Message
		maxLen := 40
		if len(msg) > maxLen {
			msg = msg[:maxLen-3] + "..."
		}

		sb.WriteString(style.Render(icon + msg))
		sb.WriteString("\n")
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

func (m Model) renderHistory() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("No changes yet...\nWaiting for Claude edits")
	}

	var sb strings.Builder
	sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("History (%d)\n", len(m.changes))))
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 20)) + "\n")

	// Calculate available width for path in history pane
	historyWidth := m.width / 3
	pathWidth := historyWidth - 15 // Account for timestamp, tool, prefix

	// Track current commit for grouping
	currentCommit := ""

	for i, change := range m.changes {
		// Show commit header when commit changes
		if change.CommitShort != "" && change.CommitShort != currentCommit {
			currentCommit = change.CommitShort
			vcsLabel := change.VCSType
			if vcsLabel == "" {
				vcsLabel = "commit"
			}
			commitHeader := fmt.Sprintf("── %s:%s ──", vcsLabel, change.CommitShort)
			sb.WriteString(m.theme.DiffHeader.Render(commitHeader) + "\n")
		}

		var line string
		if i == m.selectedIndex {
			// Selected: show scrollable relative path
			path := relativePath(change.FilePath)
			if m.scrollX > 0 && len(path) > m.scrollX {
				path = path[m.scrollX:]
			}
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				path)
			sb.WriteString(m.theme.Selected.Render("> "+line) + "\n")
		} else {
			// Not selected: truncate path
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				truncatePath(change.FilePath, pathWidth))
			sb.WriteString(m.theme.Normal.Render("  "+line) + "\n")
		}
	}

	return sb.String()
}

// renderPromptsList renders the prompts list for the left pane
func (m Model) renderPromptsList() string {
	var sb strings.Builder
	listWidth := m.width / 3

	if m.promptShowVersions {
		// Version view mode
		sb.WriteString(m.theme.Title.Render("Versions") + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n")

		if len(m.promptList) > 0 {
			p := m.promptList[m.promptSelected]
			sb.WriteString(m.theme.Dim.Render(p.Name) + "\n\n")
		}

		if len(m.promptVersions) == 0 {
			sb.WriteString(m.theme.Dim.Render("No versions found"))
		} else {
			for i, v := range m.promptVersions {
				prefix := "  "
				if i == m.promptVersionSelected {
					prefix = "> "
				}
				line := fmt.Sprintf("%sv%d", prefix, v.Version)
				if i == m.promptVersionSelected {
					sb.WriteString(m.theme.Selected.Render(line) + "\n")
				} else {
					sb.WriteString(m.theme.Normal.Render(line) + "\n")
				}
			}
		}
	} else {
		// Normal prompt list mode
		sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Prompts (%d)", len(m.promptList))) + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n")

		if len(m.promptList) == 0 {
			sb.WriteString(m.theme.Dim.Render("No prompts\nPress 'n' to create"))
		} else {
			for i, p := range m.promptList {
				prefix := "  "
				if i == m.promptSelected {
					prefix = "> "
				}
				// Show [G] for global, [P] for project
				scope := "[P]"
				if p.IsGlobal {
					scope = "[G]"
				}
				// Show version backup count
				versionStr := ""
				if p.VersionCount > 0 {
					versionStr = fmt.Sprintf(" (%d)", p.VersionCount)
				}
				line := fmt.Sprintf("%s%s %s%s", prefix, scope, p.Name, versionStr)
				if len(line) > listWidth-4 {
					line = line[:listWidth-7] + "..."
				}
				if i == m.promptSelected {
					sb.WriteString(m.theme.Selected.Render(line) + "\n")
				} else {
					sb.WriteString(m.theme.Normal.Render(line) + "\n")
				}
			}
		}
	}

	return sb.String()
}

func (m *Model) renderDiff() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("Select a change to view diff")
	}

	// Use cache if available and no horizontal scroll
	if m.scrollX == 0 {
		if cached, ok := m.diffCache[m.selectedIndex]; ok {
			return cached
		}
	}

	change := m.changes[m.selectedIndex]

	// If FileContent is empty (e.g., loaded from history), try to read the file
	if change.FileContent == "" && change.FilePath != "" && change.ToolName != "Write" {
		if content, err := os.ReadFile(change.FilePath); err == nil {
			change.FileContent = string(content)
			// Update the stored change so we don't re-read every time
			m.changes[m.selectedIndex] = change
			logger.Log("Re-read file content for history entry: %s (%d bytes)", change.FilePath, len(change.FileContent))
		} else {
			logger.Log("Failed to re-read file for history entry: %s: %v", change.FilePath, err)
		}
	}

	var sb strings.Builder

	// Header with relative file path
	sb.WriteString(m.theme.Title.Render(relativePath(change.FilePath)))
	if change.LineNum > 0 {
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf(":%d", change.LineNum)))
	}
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// If we have file content, show full file with change highlighted
	if change.FileContent != "" && change.ToolName != "Write" {
		sb.WriteString(m.renderFileWithChange(change))
	} else if change.ToolName == "Write" {
		// For Write operations, show highlighted new content
		content := change.NewString
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		sb.WriteString(m.theme.DiffHeader.Render("@@ New file @@"))
		sb.WriteString("\n\n")

		lines := diff.SplitLines(content)
		for i, line := range lines {
			lineNum := fmt.Sprintf("%4d", i+1)
			highlighted := m.highlighter.HighlightLine(line, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Added.Render("+ "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	} else if change.OldString != "" || change.NewString != "" {
		// Fallback: show just the diff
		opts := diff.DefaultOptions()
		diffOutput := diff.FormatDiff(change.OldString, change.NewString, m.theme, opts)
		sb.WriteString(diffOutput)
	} else {
		sb.WriteString(m.theme.Dim.Render("No diff content available"))
	}

	return sb.String()
}

// renderRightPane returns the content for the right pane based on current mode
func (m *Model) renderRightPane() string {
	// Show chat pane if chat is active
	if m.chatActive {
		return m.renderChatPane()
	}

	switch m.leftPaneMode {
	case LeftPaneModePrompts:
		return m.renderPromptPreview()
	case LeftPaneModeRalph:
		return m.renderRalphPrompt()
	case LeftPaneModePlan:
		return m.renderPlanContent()
	default:
		return m.renderDiff()
	}
}

// renderRalphPrompt renders the Ralph prompt content for the right pane
func (m *Model) renderRalphPrompt() string {
	var sb strings.Builder

	if m.ralphState == nil || !m.ralphState.Active {
		return m.theme.Dim.Render("No active Ralph loop.\n\nSwitch to History or Prompts tab to work on something else.")
	}

	sb.WriteString(m.theme.Title.Render("Ralph Loop Prompt") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	if m.ralphState.Prompt == "" {
		sb.WriteString(m.theme.Dim.Render("No prompt content found"))
		return sb.String()
	}

	// Render prompt as markdown
	rendered, err := m.renderMarkdown(m.ralphState.Prompt, m.diffViewport.Width-4)
	if err != nil {
		sb.WriteString(m.ralphState.Prompt)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}

// renderPlanContent renders the plan content for the right pane
func (m *Model) renderPlanContent() string {
	var sb strings.Builder

	if m.planPath == "" || m.planContent == "" {
		return m.theme.Dim.Render("No active plan.\n\nPlans are created when Claude enters plan mode.")
	}

	planName := strings.TrimSuffix(filepath.Base(m.planPath), ".md")
	sb.WriteString(m.theme.Title.Render(planName) + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// Render plan as markdown
	rendered, err := m.renderMarkdown(m.planContent, m.diffViewport.Width-4)
	if err != nil {
		sb.WriteString(m.planContent)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}

// renderPromptPreview renders the prompt preview for the right pane in prompts mode
func (m *Model) renderPromptPreview() string {
	var sb strings.Builder

	if m.promptShowVersions {
		// Version preview mode
		if len(m.promptVersions) == 0 {
			return m.theme.Dim.Render("No versions available")
		}

		v := m.promptVersions[m.promptVersionSelected]
		content, err := os.ReadFile(v.Path)
		if err != nil {
			return m.theme.Dim.Render("Failed to read version: " + err.Error())
		}

		// Parse and show the content
		p, err := prompt.Parse(string(content))
		if err != nil {
			return string(content)
		}

		sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Version %d", v.Version)) + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

		// Render as markdown
		rendered, err := m.renderMarkdown(p.Content, m.diffViewport.Width-4)
		if err != nil {
			sb.WriteString(p.Content)
		} else {
			sb.WriteString(rendered)
		}
		return sb.String()
	}

	// Normal prompt preview
	if len(m.promptList) == 0 {
		return m.theme.Dim.Render("No prompts yet.\n\nPress 'n' to create a new prompt.\nPress 'o' to switch back to History mode.")
	}

	p := m.promptList[m.promptSelected]

	// Header
	sb.WriteString(m.theme.Title.Render(p.Name) + "\n")
	if p.Description != "" && p.Description != "Describe what this prompt does" {
		sb.WriteString(m.theme.Dim.Render(p.Description) + "\n")
	}
	sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("v%d | %s | %s", p.Version, p.Updated.Format("2006-01-02"), prompt.MethodName(m.promptInjectMethod))) + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// Render content as markdown
	rendered, err := m.renderMarkdown(p.Content, m.diffViewport.Width-4)
	if err != nil {
		sb.WriteString(p.Content)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}

// renderFileWithChange shows the full file with the changed section highlighted
func (m *Model) renderFileWithChange(change Change) string {
	var sb strings.Builder

	// Split file content into lines
	fileLines := diff.SplitLines(change.FileContent)
	oldLines := diff.SplitLines(change.OldString)
	newLines := diff.SplitLines(change.NewString)

	changeStart := change.LineNum - 1 // 0-indexed
	changeEnd := changeStart + len(oldLines)

	// Track total lines for minimap
	m.totalLines = len(fileLines) + len(newLines)

	// Build minimap data
	m.minimapData = minimap.New(m.totalLines)
	// Mark removed lines
	m.minimapData.SetRange(changeStart, changeEnd, minimap.LineRemoved)
	// Mark added lines (they appear after the removed lines in the view)
	// The added lines logically replace the removed ones at changeStart
	for i := 0; i < len(newLines); i++ {
		m.minimapData.SetLine(changeStart+i, minimap.LineAdded)
	}

	// Show diff header with stats
	sb.WriteString(m.theme.DiffHeader.Render(fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		change.LineNum, len(oldLines), change.LineNum, len(newLines))))
	sb.WriteString("  ")
	sb.WriteString(m.theme.Added.Render(fmt.Sprintf("+%d", len(newLines))))
	sb.WriteString(" ")
	sb.WriteString(m.theme.Removed.Render(fmt.Sprintf("-%d", len(oldLines))))
	sb.WriteString("\n\n")

	// Soft highlight style for changed lines
	changedBg := lipgloss.NewStyle().Background(m.theme.ChangedLineBg)

	// Render the entire file
	for i := 0; i < len(fileLines); i++ {
		lineNum := fmt.Sprintf("%4d", i+1)
		line := fileLines[i]

		// Apply horizontal scroll
		scrolledLine := line
		if m.scrollX > 0 && len(line) > m.scrollX {
			scrolledLine = line[m.scrollX:]
		} else if m.scrollX > 0 {
			scrolledLine = ""
		}

		// Check if this line is in the changed region
		if i >= changeStart && i < changeEnd {
			// This is a removed line - use diff colors (no syntax highlighting)
			lineContent := m.theme.LineNumberActive.Render(lineNum) + " " +
				m.theme.Removed.Render("- "+scrolledLine)
			sb.WriteString(changedBg.Render(lineContent))
			sb.WriteString("\n")

			// After the last removed line, insert the new lines
			if i == changeEnd-1 {
				for j, newLine := range newLines {
					// Apply horizontal scroll to new lines too
					scrolledNew := newLine
					if m.scrollX > 0 && len(newLine) > m.scrollX {
						scrolledNew = newLine[m.scrollX:]
					} else if m.scrollX > 0 {
						scrolledNew = ""
					}

					newLineNum := fmt.Sprintf("%4d", changeStart+j+1)
					lineContent := m.theme.LineNumberActive.Render(newLineNum) + " " +
						m.theme.Added.Render("+ "+scrolledNew)
					sb.WriteString(changedBg.Render(lineContent))
					sb.WriteString("\n")
				}
			}
		} else {
			// Context line - use syntax highlighting
			highlighted := m.highlighter.HighlightLine(scrolledLine, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Context.Render("  "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// scrollToChange scrolls the viewport to show the current change
func (m *Model) scrollToChange() {
	if len(m.changes) == 0 {
		return
	}
	change := m.changes[m.selectedIndex]
	// Scroll to a few lines before the change so it's visible in context
	// Add 2 for the header lines in the diff view
	targetLine := change.LineNum - 3
	if targetLine < 0 {
		targetLine = 0
	}
	m.diffViewport.SetYOffset(targetLine)
}

// preloadAdjacent pre-caches rendered diffs for adjacent changes
func (m *Model) preloadAdjacent() {
	// Preload next
	if m.selectedIndex+1 < len(m.changes) {
		idx := m.selectedIndex + 1
		if _, ok := m.diffCache[idx]; !ok {
			// Store current state
			origIdx := m.selectedIndex
			origScrollX := m.scrollX
			// Render next
			m.selectedIndex = idx
			m.scrollX = 0
			m.diffCache[idx] = m.renderDiff()
			// Restore
			m.selectedIndex = origIdx
			m.scrollX = origScrollX
		}
	}
	// Preload previous
	if m.selectedIndex > 0 {
		idx := m.selectedIndex - 1
		if _, ok := m.diffCache[idx]; !ok {
			origIdx := m.selectedIndex
			origScrollX := m.scrollX
			m.selectedIndex = idx
			m.scrollX = 0
			m.diffCache[idx] = m.renderDiff()
			m.selectedIndex = origIdx
			m.scrollX = origScrollX
		}
	}
}

// updateViewportSize updates the viewport dimensions based on current layout
func (m *Model) updateViewportSize() {
	headerHeight := 2
	footerHeight := 1
	minimapWidth := 0
	if m.showMinimap {
		minimapWidth = 2
	}

	// Always two-pane layout
	leftWidth := m.width / 3
	vpWidth := m.width - leftWidth - 6 - minimapWidth

	m.diffViewport.Width = vpWidth
	m.diffViewport.Height = m.height - headerHeight - footerHeight - 2
}

// renderMinimap renders a visual minimap showing file structure and diff regions
func (m Model) renderMinimap() string {
	if !m.showMinimap {
		return ""
	}

	height := m.height - 4
	if height < 3 {
		return ""
	}

	// If we have minimap data, use the visual minimap
	if m.minimapData != nil && m.minimapData.TotalLines() > 0 {
		viewportStart := m.diffViewport.YOffset
		viewportEnd := viewportStart + m.diffViewport.Height
		return m.minimapData.Render(height, viewportStart, viewportEnd, m.theme)
	}

	// Fallback: simple scrollbar if no minimap data
	var sb strings.Builder

	totalLines := m.totalLines
	if totalLines < 1 {
		totalLines = 1
	}

	viewportHeight := m.diffViewport.Height
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Thumb size proportional to viewport vs total content
	thumbSize := (viewportHeight * height) / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}

	// Thumb position based on scroll offset
	scrollPos := m.diffViewport.YOffset
	maxScroll := totalLines - viewportHeight
	if maxScroll < 1 {
		maxScroll = 1
	}

	thumbPos := (scrollPos * (height - thumbSize)) / maxScroll
	if thumbPos < 0 {
		thumbPos = 0
	}
	if thumbPos+thumbSize > height {
		thumbPos = height - thumbSize
	}

	trackStyle := lipgloss.NewStyle().Foreground(m.theme.ScrollbarBg)
	thumbStyle := lipgloss.NewStyle().Foreground(m.theme.ScrollbarThumb)

	for i := 0; i < height; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(thumbStyle.Render("▐▐"))
		} else {
			sb.WriteString(trackStyle.Render("░░"))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderStatus() string {
	// Chat mode has its own hints
	if m.chatActive {
		return m.theme.Status.Render("Enter:send  Esc:close  ^C:kill  ^L:clear")
	}

	minimapKey := "m:map"
	if m.showMinimap {
		minimapKey = "m:map✓"
	}

	// Common tab navigation hints
	tabHints := "Tab:next  [/]:pane  1-4:tabs  c:chat"

	var keys string
	switch m.leftPaneMode {
	case LeftPaneModePrompts:
		if m.promptShowVersions {
			// Version view mode hints
			keys = fmt.Sprintf("j/k:nav  r/Enter:revert  e:view  ^D:del  V/esc:back  %s  ?:help  q:quit", tabHints)
		} else {
			// Prompts mode hints
			methodName := prompt.MethodName(m.promptInjectMethod)
			keys = fmt.Sprintf("j/k:nav  n:new  e:edit  r:refine  v:ver  V:versions  ^D:del  y:yank  i:(%s)  Enter:send  %s  ?:help  q:quit", methodName, tabHints)
		}
	case LeftPaneModeRalph:
		// Ralph mode hints
		if m.ralphState != nil && m.ralphState.Active {
			keys = fmt.Sprintf("j/k:scroll  C:cancel  r:refresh  %s  ?:help  q:quit", tabHints)
		} else {
			keys = fmt.Sprintf("r:refresh  %s  ?:help  q:quit", tabHints)
		}
	case LeftPaneModePlan:
		// Plan mode hints
		if m.planInputActive {
			keys = "Enter:submit  Esc:cancel"
		} else if m.planGenerating {
			keys = "Generating plan..."
		} else if m.planPath != "" {
			keys = fmt.Sprintf("j/k:scroll  d/u:page  G:new  e:edit  r:refresh  %s  ?:help  q:quit", tabHints)
		} else {
			keys = fmt.Sprintf("G:generate  r:refresh  %s  ?:help  q:quit", tabHints)
		}
	default:
		// History mode hints
		keys = fmt.Sprintf("n/p:nav  ←→:scroll  %s  ^G:nvim  c:clear  %s  ?:help  q:quit", minimapKey, tabHints)
	}
	return m.theme.Status.Render(keys)
}

func (m Model) renderHelp() string {
	help := `
  Claude Follow TUI - Help

  Tab Navigation:
    Tab          Cycle to next tab
    Shift+Tab    Cycle to previous tab
    1            History tab
    2            Prompts tab
    3            Ralph tab
    4            Plan tab
    [ / ]        Switch left/right pane focus

  History Tab:
    n/p          Next/previous change
    j/k          Scroll diff
    ←/→          Scroll horizontally
    m            Toggle minimap
    Ctrl+G       Open file in nvim at line
    Ctrl+O       Open file in nvim
    c            Clear history

  Prompts Tab:
    n            New prompt
    e            Edit selected prompt
    r            Refine with Claude CLI
    v            Create version backup
    V            View all versions
    Ctrl+d       Delete prompt
    y            Yank (copy to clipboard)
    i            Cycle inject method
    Enter        Inject prompt

  Version View:
    j/k          Navigate versions
    r/Enter      Revert to version
    e            View version (read-only)
    Ctrl+d       Delete version
    V/esc        Back to prompts

  Ralph Tab:
    C            Cancel Ralph loop
    r            Refresh status

  Plan Tab:
    G            Generate new plan
    e            Edit plan in nvim
    r            Refresh plan
    d/u          Page down/up

  Chat (press 'c' from any tab):
    Enter        Send message
    Esc          Close chat
    Ctrl+C       Kill chat and close
    Ctrl+L       Clear output

  Template Variables (in prompts):
    {{plan}}       Plan file content
    {{plan_name}}  Plan file name
    {{file}}       Current file path
    {{file_name}}  Current file name
    {{project}}    Project directory name
    {{cwd}}        Working directory

  ?            Show this help
  q            Quit

  Press any key to close help
`
	return m.theme.Help.Render(help)
}

func parsePayload(data []byte) *Change {
	logger.Log("parsePayload: raw data: %s", string(data))

	var payload HookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		logger.Log("parsePayload: JSON unmarshal error: %v", err)
		return nil
	}

	logger.Log("parsePayload: tool_name=%s", payload.ToolName)

	// Extract file path (try multiple locations)
	filePath := payload.ToolInput.FilePath
	if filePath == "" {
		filePath = payload.ToolInput.Path
	}
	if filePath == "" {
		filePath = payload.Parameters.FilePath
	}
	if filePath == "" {
		filePath = payload.Parameters.Path
	}
	logger.Log("parsePayload: filePath=%s", filePath)
	if filePath == "" {
		logger.Log("parsePayload: filePath empty, returning nil")
		return nil
	}

	// Extract old/new strings
	oldStr := payload.ToolInput.OldString
	if oldStr == "" {
		oldStr = payload.Parameters.OldString
	}

	newStr := payload.ToolInput.NewString
	if newStr == "" {
		newStr = payload.Parameters.NewString
	}
	if newStr == "" {
		newStr = payload.ToolInput.Content
	}

	// Read the full file content
	var fileContent string
	var lineNum int = 1
	var lineCount int = 1

	if content, err := os.ReadFile(filePath); err == nil {
		fileContent = string(content)
		logger.Log("parsePayload: read file successfully, %d bytes", len(fileContent))

		// Find line number where the change occurs
		if oldStr != "" {
			lineNum = findLineNumber(fileContent, oldStr)
			lineCount = strings.Count(oldStr, "\n") + 1
		} else if newStr != "" {
			// For Write operations, show from beginning
			lineNum = 1
			lineCount = strings.Count(newStr, "\n") + 1
		}
	} else {
		logger.Log("parsePayload: failed to read file %s: %v", filePath, err)
	}

	return &Change{
		Timestamp:   time.Now(),
		FilePath:    filePath,
		ToolName:    payload.ToolName,
		OldString:   oldStr,
		NewString:   newStr,
		FileContent: fileContent,
		LineNum:     lineNum,
		LineCount:   lineCount,
	}
}

// findLineNumber finds the line number where searchStr first appears in content
func findLineNumber(content, searchStr string) int {
	if searchStr == "" {
		return 1
	}

	idx := strings.Index(content, searchStr)
	if idx == -1 {
		return 1
	}

	// Count newlines before the match
	return strings.Count(content[:idx], "\n") + 1
}

func truncatePath(path string, maxLen int) string {
	// First make it relative
	path = relativePath(path)
	if len(path) <= maxLen {
		return path
	}
	// Show last part of path
	parts := strings.Split(path, "/")
	result := parts[len(parts)-1]
	if len(result) > maxLen {
		return "..." + result[len(result)-maxLen+3:]
	}
	return ".../" + result
}

// relativePath converts an absolute path to relative if possible
func relativePath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	// If relative path starts with many ../, just use filename
	if strings.HasPrefix(rel, "../../..") {
		return filepath.Base(path)
	}
	return rel
}

// cwdToProjectDir converts a CWD path to Claude's project directory name
// /Users/foo/bar.baz → -Users-foo-bar-baz
func cwdToProjectDir(cwd string) string {
	result := strings.ReplaceAll(cwd, "/", "-")
	result = strings.ReplaceAll(result, ".", "-")
	return result
}

// extractSlugFromJSONL reads the slug field from the last entry in a JSONL file
func extractSlugFromJSONL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Seek to end and read last chunk (avoid loading huge file)
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	size := stat.Size()
	readSize := int64(4096)
	if size < readSize {
		readSize = size
	}

	if _, err := f.Seek(-readSize, io.SeekEnd); err != nil {
		// If seek fails, try reading from start
		f.Seek(0, io.SeekStart)
	}

	buf := make([]byte, readSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return ""
	}

	// Find last complete JSON line with slug
	lines := strings.Split(string(buf[:n]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse JSON and extract slug
		var entry struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Slug != "" {
			return entry.Slug
		}
	}
	return ""
}

// findPlanFromSession looks up the plan file for the current session
func (m *Model) findPlanFromSession(home string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	projectDir := cwdToProjectDir(cwd)
	projectPath := filepath.Join(home, ".claude", "projects", projectDir)

	// Find most recent .jsonl in project directory
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return ""
	}

	var newestJSONL string
	var newestTime time.Time
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil || info == nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestJSONL = filepath.Join(projectPath, e.Name())
		}
	}

	if newestJSONL == "" {
		return ""
	}

	// Extract slug from JSONL
	slug := extractSlugFromJSONL(newestJSONL)
	if slug == "" {
		return ""
	}

	// Construct plan path and verify it exists
	planPath := filepath.Join(home, ".claude", "plans", slug+".md")
	if _, err := os.Stat(planPath); err == nil {
		return planPath
	}
	return ""
}

// findMostRecentPlan finds the most recently modified plan file (fallback)
func (m *Model) findMostRecentPlan(home string) string {
	plansDir := filepath.Join(home, ".claude", "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return ""
	}

	var newestPath string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(plansDir, entry.Name())
		}
	}
	return newestPath
}

// loadPlanFile finds and loads the active Claude plan file
// Priority: 1) Path from hook, 2) Session-aware lookup, 3) Most recent plan
func (m *Model) loadPlanFile() {
	m.planContent = ""

	// Use path from hook if already set and valid
	planPath := m.planPath
	if planPath != "" {
		if content, err := os.ReadFile(planPath); err == nil {
			m.planContent = string(content)
			return
		}
		// Path invalid, clear it and try other methods
		m.planPath = ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Try session-aware lookup
	planPath = m.findPlanFromSession(home)

	// Fallback to most recent plan
	if planPath == "" {
		planPath = m.findMostRecentPlan(home)
	}

	if planPath == "" {
		m.planContent = "No plan files found in ~/.claude/plans/"
		return
	}

	// Read the plan file
	content, err := os.ReadFile(planPath)
	if err != nil {
		m.planContent = fmt.Sprintf("Error reading plan: %v", err)
		return
	}

	m.planPath = planPath
	m.planContent = string(content)
}

// renderMarkdown renders markdown content using glamour
func (m Model) renderMarkdown(content string, width int) (string, error) {
	// Choose style based on current theme
	style := styles.DarkStyleConfig
	if m.theme.Name == "light" {
		style = styles.LightStyleConfig
	}

	// Create renderer with the appropriate style and width
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}

	rendered, err := r.Render(content)
	if err != nil {
		return "", err
	}

	// Trim trailing whitespace
	return strings.TrimRight(rendered, "\n"), nil
}

// refreshPromptList reloads the list of prompts from storage
func (m *Model) refreshPromptList() {
	if m.promptStore == nil {
		return
	}
	prompts, err := m.promptStore.List()
	if err != nil {
		logger.Log("Failed to list prompts: %v", err)
		return
	}
	m.promptList = prompts
	if m.promptSelected >= len(m.promptList) {
		m.promptSelected = 0
	}
}

// createNewPrompt creates a new prompt file and opens it in nvim
func (m *Model) createNewPrompt(isGlobal bool) (Model, tea.Cmd) {
	if m.promptStore == nil {
		return *m, nil
	}

	// Create temp file with template
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "new-prompt.prompt.md")
	template := prompt.NewPromptTemplate("New Prompt")
	if err := os.WriteFile(tmpPath, []byte(template), 0644); err != nil {
		logger.Log("Failed to create temp prompt: %v", err)
		return *m, nil
	}

	cmd := exec.Command("nvim", tmpPath)
	return *m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return nil
		}
		// Read the edited content and save to prompts directory
		content, err := os.ReadFile(tmpPath)
		if err != nil {
			return nil
		}
		os.Remove(tmpPath)

		// Parse and save
		p, err := prompt.Parse(string(content))
		if err != nil {
			return nil
		}
		// Set scope based on isGlobal flag
		p.IsGlobal = isGlobal

		store, err := prompt.NewStore()
		if err != nil {
			return nil
		}
		if err := store.Save(p); err != nil {
			logger.Log("Failed to save new prompt: %v", err)
			return nil
		}
		return promptEditedMsg{path: p.Path}
	})
}

// editPrompt opens an existing prompt in nvim for editing
func (m *Model) editPrompt(p prompt.Prompt) (Model, tea.Cmd) {
	// Auto-create version backup before editing
	if m.promptStore != nil {
		if err := m.promptStore.CreateVersion(&p); err != nil {
			logger.Log("Failed to create version before edit: %v", err)
		} else {
			logger.Log("Created version backup before edit: v%d", p.Version)
		}
	}

	cmd := exec.Command("nvim", p.Path)
	return *m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return promptEditedMsg{path: p.Path}
	})
}

// refinePrompt uses Claude CLI to refine a prompt, then opens nvim diff
func (m *Model) refinePrompt(p prompt.Prompt) (Model, tea.Cmd) {
	m.promptRefining = true

	// Auto-create version backup before refining
	if m.promptStore != nil {
		if err := m.promptStore.CreateVersion(&p); err != nil {
			logger.Log("Failed to create version before refine: %v", err)
		} else {
			logger.Log("Created version backup v%d before refine", p.Version)
		}
	}

	return *m, func() tea.Msg {
		// Build refinement meta-prompt
		metaPrompt := fmt.Sprintf(`You are a prompt engineering expert. Improve the following prompt to be clearer, more specific, and more effective.

Keep the same general intent but enhance:
- Clarity of instructions
- Specificity of expected output format
- Edge case handling
- Any missing context that would help

<current_prompt>
%s
</current_prompt>

Output ONLY the improved prompt text with no preamble, no explanations, no markdown code fences. Just the raw improved prompt.`, p.Content)

		// Run Claude CLI with -p flag for print mode (non-interactive)
		cmd := exec.Command("claude", "-p", metaPrompt)
		cmd.Env = append(os.Environ(), "CLAUDE_CODE_ENTRYPOINT=cli")
		output, err := cmd.Output()
		if err != nil {
			// Try to get stderr for better error message
			if exitErr, ok := err.(*exec.ExitError); ok {
				return promptRefineErrorMsg{err: fmt.Errorf("claude CLI failed: %s", string(exitErr.Stderr))}
			}
			return promptRefineErrorMsg{err: fmt.Errorf("claude CLI failed: %w", err)}
		}

		// Clean up output - trim whitespace and any markdown code fences
		refined := strings.TrimSpace(string(output))
		refined = strings.TrimPrefix(refined, "```")
		refined = strings.TrimSuffix(refined, "```")
		refined = strings.TrimSpace(refined)

		// Create a new prompt with the refined content but same metadata
		refinedPrompt := prompt.Prompt{
			Name:        p.Name,
			Description: p.Description,
			Version:     p.Version,
			Created:     p.Created,
			Updated:     p.Updated,
			Tags:        p.Tags,
			Content:     refined,
		}

		// Write refined version to temp file with frontmatter
		refinedPath := filepath.Join(os.TempDir(), "refined-"+filepath.Base(p.Path))
		if err := os.WriteFile(refinedPath, []byte(refinedPrompt.Format()), 0644); err != nil {
			return promptRefineErrorMsg{err: fmt.Errorf("failed to write refined prompt: %w", err)}
		}

		return promptRefinedMsg{
			originalPath: p.Path,
			refinedPath:  refinedPath,
		}
	}
}

// loadVersionList loads the list of versions for the currently selected prompt
func (m *Model) loadVersionList() {
	if m.promptStore == nil || len(m.promptList) == 0 {
		m.promptVersions = nil
		return
	}

	p := m.promptList[m.promptSelected]
	versions, err := m.promptStore.ListVersions(p.Path)
	if err != nil {
		logger.Log("Failed to list versions: %v", err)
		m.promptVersions = nil
		return
	}
	m.promptVersions = versions
}

// expandPromptVariables replaces template variables in prompt content
// Supported variables:
//   - {{plan}} - Current plan file content
//   - {{plan_name}} - Plan file name
//   - {{file}} - Current selected file path
//   - {{file_name}} - Current selected file name
//   - {{project}} - Current project/directory name
//   - {{cwd}} - Current working directory
func (m *Model) expandPromptVariables(content string) string {
	// Determine project directory - prefer deriving from file paths
	var projectDir string

	// Get current file info from history
	var filePath, fileName string
	if len(m.changes) > 0 && m.selectedIndex < len(m.changes) {
		filePath = m.changes[m.selectedIndex].FilePath
		fileName = filepath.Base(filePath)
		// Try to find project root by looking for .git
		projectDir = findProjectRoot(filepath.Dir(filePath))
	}

	// Fall back to prompt store's project dir, then cwd
	if projectDir == "" && m.promptStore != nil {
		// The prompt store knows the project directory
		projectDir, _ = os.Getwd()
	}
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	projectName := filepath.Base(projectDir)

	// Get plan info
	planName := ""
	planContent := ""
	if m.planPath != "" {
		planName = filepath.Base(m.planPath)
		if data, err := os.ReadFile(m.planPath); err == nil {
			planContent = string(data)
		}
	}

	logger.Log("expandPromptVariables: projectDir=%s, project=%s, file=%s, fileName=%s, planPath=%s",
		projectDir, projectName, filePath, fileName, m.planPath)

	// Replace variables
	result := content
	result = strings.ReplaceAll(result, "{{plan}}", planContent)
	result = strings.ReplaceAll(result, "{{plan_name}}", planName)
	result = strings.ReplaceAll(result, "{{file}}", filePath)
	result = strings.ReplaceAll(result, "{{file_name}}", fileName)
	result = strings.ReplaceAll(result, "{{project}}", projectName)
	result = strings.ReplaceAll(result, "{{cwd}}", projectDir)

	return result
}

// findProjectRoot walks up from dir looking for .git directory
func findProjectRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, return original
			return ""
		}
		dir = parent
	}
}
