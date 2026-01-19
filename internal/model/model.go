package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/ztaylor/claude-mon/internal/config"
	workingctx "github.com/ztaylor/claude-mon/internal/context"
	"github.com/ztaylor/claude-mon/internal/diff"
	"github.com/ztaylor/claude-mon/internal/highlight"
	"github.com/ztaylor/claude-mon/internal/history"
	"github.com/ztaylor/claude-mon/internal/logger"
	"github.com/ztaylor/claude-mon/internal/minimap"
	"github.com/ztaylor/claude-mon/internal/plan"
	"github.com/ztaylor/claude-mon/internal/prompt"
	"github.com/ztaylor/claude-mon/internal/ralph"
	"github.com/ztaylor/claude-mon/internal/theme"
	"github.com/ztaylor/claude-mon/internal/vcs"
)

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
	LeftPaneModeContext
)

// PromptFilter defines the scope filter for prompts
type PromptFilter int

const (
	PromptFilterAll     PromptFilter = iota // Show all prompts
	PromptFilterProject                     // Show only project prompts
	PromptFilterGlobal                      // Show only global prompts
)

// Model is the Bubbletea model
type Model struct {
	socketPath       string
	socketConnected  bool      // Whether socket is listening
	lastMsgTime      time.Time // Time of last received message
	width            int
	height           int
	activePane       Pane
	leftPaneMode     LeftPaneMode // History or Prompts mode
	changes          []Change
	selectedIndex    int
	diffViewport     viewport.Model
	showHelp         bool
	showMinimap      bool // Toggle minimap visibility
	planContent      string
	planPath         string
	planViewport     viewport.Model
	ready            bool
	theme            *theme.Theme
	highlighter      *highlight.Highlighter
	scrollX          int              // Horizontal scroll offset
	listScrollOffset int              // Vertical scroll offset for history list
	totalLines       int              // Total lines in current file (for minimap)
	minimapData      *minimap.Minimap // Cached minimap line types
	diffCache        map[int]string   // Cached rendered diffs by index
	historyStore     *history.Store   // Persistent history storage
	persistHistory   bool             // Whether to save history to file

	// Prompt manager (integrated in left pane)
	promptStore         *prompt.Store          // Prompt storage
	promptList          []prompt.Prompt        // Cached list of prompts (all prompts)
	promptFilteredList  []prompt.Prompt        // Filtered list based on scope
	promptSelected      int                    // Selected prompt index
	promptFilter        PromptFilter           // Current filter scope (all/project/global)
	promptFuzzyActive   bool                   // Whether fuzzy filter overlay is active
	promptFuzzyInput    textinput.Model        // Fuzzy search input
	promptFuzzyMatches  []int                  // Indices of matching prompts
	promptFuzzySelected int                    // Selected match in fuzzy results
	promptInjectMethod  prompt.InjectionMethod // Current injection method

	// Version view mode
	promptShowVersions    bool                   // Whether showing version list
	promptVersions        []prompt.PromptVersion // List of versions for selected prompt
	promptVersionSelected int                    // Selected version index

	// Toast notifications
	toasts []Toast // Active toast notifications

	// Ralph mode state
	ralphState      *ralph.State
	ralphRefreshCmd tea.Cmd // Ticker for auto-refreshing Ralph state

	// Plan generation
	planInputActive bool            // Whether plan input is active
	planInput       textinput.Model // Plan description input
	planGenerating  bool            // Whether plan is being generated

	// Context management
	contextCurrent   *workingctx.Context   // Current project context
	contextList      []*workingctx.Context // All project contexts
	contextSelected  int                   // Selected context in list view
	contextShowList  bool                  // Whether to show all contexts list
	contextEditMode  bool                  // Whether editing context values
	contextEditField string                // Which context type: k8s, aws, git, env, custom
	contextViewport  viewport.Model

	// Multi-field inputs for context editing
	k8sKubeconfigInput textinput.Model // Kubeconfig file path
	k8sContextInput    textinput.Model // Context name
	k8sNamespaceInput  textinput.Model // Namespace
	k8sFocusedField    int             // 0=kubeconfig, 1=context, 2=namespace

	gitBranchInput  textinput.Model // Branch name
	gitRepoInput    textinput.Model // Repository name
	gitFocusedField int             // 0=branch, 1=repo

	awsProfileInput textinput.Model // AWS profile
	awsRegionInput  textinput.Model // AWS region
	awsFocusedField int             // 0=profile, 1=region

	envInput    textinput.Model // KEY=VALUE for env
	customInput textinput.Model // KEY=VALUE for custom

	// Context completion (in-app fuzzy search)
	contextCompletionActive     bool            // Whether completion overlay is showing
	contextCompletionInput      textinput.Model // Filter input for completion
	contextCompletionCandidates []string        // All candidates for current field
	contextCompletionMatches    []int           // Indices of matching candidates
	contextCompletionSelected   int             // Currently selected match index

	// Layout
	hideLeftPane bool // Toggle left pane visibility

	// Leader key / which-key state
	leaderActive      bool      // Whether leader popup is showing
	leaderActivatedAt time.Time // When leader mode was activated (for timeout)

	// Configuration
	config *config.Config // User configuration

	// Keybindings (bubbles/key integration)
	keyMap KeyMap     // KeyMap with help text for bubbles/help
	help   help.Model // bubbles/help for rendering keybinding help

	// Daemon connection status
	daemonConnected       bool      // Whether daemon is reachable
	daemonUptime          string    // Daemon uptime string
	daemonLastCheck       time.Time // Last time we checked daemon status
	daemonWorkspaceActive bool      // Whether current workspace has activity
	daemonWorkspaceEdits  int       // Edit count for current workspace
	daemonLastActivity    time.Time // Last activity time for current workspace
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

// WithConfig sets a custom configuration for the model
func WithConfig(cfg *config.Config) Option {
	return func(m *Model) {
		m.config = cfg
	}
}

// New creates a new Model with optional configuration
func New(socketPath string, opts ...Option) Model {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Log("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}

	// Get theme from config
	t := theme.Get(cfg.Theme)
	if t == nil {
		t = theme.Default()
	}

	m := Model{
		socketPath:      socketPath,
		socketConnected: socketPath != "", // Socket is listening if path provided
		changes:         []Change{},
		activePane:      PaneLeft,
		leftPaneMode:    LeftPaneModeHistory,
		showMinimap:     true,
		theme:           t,
		highlighter:     highlight.NewHighlighter(t),
		diffCache:       make(map[int]string),
		config:          cfg,
		keyMap:          FromConfig(cfg),
		help:            help.New(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	// If config was changed via option, update theme and keymap to match
	if m.config != cfg {
		cfg = m.config
		t = theme.Get(cfg.Theme)
		if t == nil {
			t = theme.Default()
		}
		m.theme = t
		m.highlighter = highlight.NewHighlighter(t)
		m.keyMap = FromConfig(cfg)
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
			// Select most recent (last) item
			if len(m.changes) > 0 {
				m.selectedIndex = len(m.changes) - 1
			}
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

	// Initialize fuzzy filter input
	fuzzyTi := textinput.New()
	fuzzyTi.Placeholder = "Type to filter..."
	fuzzyTi.CharLimit = 100
	fuzzyTi.Width = 40
	m.promptFuzzyInput = fuzzyTi

	// Initialize context
	if ctx, err := workingctx.Load(); err == nil {
		m.contextCurrent = ctx
	} else {
		logger.Log("Failed to load context: %v", err)
		m.contextCurrent = workingctx.New()
	}

	// Initialize k8s inputs
	m.k8sKubeconfigInput = textinput.New()
	m.k8sKubeconfigInput.Placeholder = "~/.kube/config"
	m.k8sKubeconfigInput.CharLimit = 200
	m.k8sKubeconfigInput.Width = 40

	m.k8sContextInput = textinput.New()
	m.k8sContextInput.Placeholder = "context name"
	m.k8sContextInput.CharLimit = 100
	m.k8sContextInput.Width = 40

	m.k8sNamespaceInput = textinput.New()
	m.k8sNamespaceInput.Placeholder = "default"
	m.k8sNamespaceInput.CharLimit = 100
	m.k8sNamespaceInput.Width = 40

	// Initialize git inputs
	m.gitBranchInput = textinput.New()
	m.gitBranchInput.Placeholder = "branch name"
	m.gitBranchInput.CharLimit = 100
	m.gitBranchInput.Width = 40

	m.gitRepoInput = textinput.New()
	m.gitRepoInput.Placeholder = "repository (auto-detected)"
	m.gitRepoInput.CharLimit = 200
	m.gitRepoInput.Width = 40

	// Initialize aws inputs
	m.awsProfileInput = textinput.New()
	m.awsProfileInput.Placeholder = "profile name"
	m.awsProfileInput.CharLimit = 100
	m.awsProfileInput.Width = 40

	m.awsRegionInput = textinput.New()
	m.awsRegionInput.Placeholder = "us-east-1"
	m.awsRegionInput.CharLimit = 50
	m.awsRegionInput.Width = 40

	// Initialize env/custom inputs
	m.envInput = textinput.New()
	m.envInput.Placeholder = `KEY="value with spaces"`
	m.envInput.CharLimit = 200
	m.envInput.Width = 40

	m.customInput = textinput.New()
	m.customInput.Placeholder = `KEY="value"`
	m.customInput.CharLimit = 200
	m.customInput.Width = 40

	// Initialize context completion input
	compTi := textinput.New()
	compTi.Placeholder = "Type to filter..."
	compTi.CharLimit = 100
	compTi.Width = 40
	m.contextCompletionInput = compTi

	// Initialize context viewport
	m.contextViewport = viewport.New(0, 0)
	m.contextViewport.GotoTop()

	return m
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	// Use tea.Batch to run multiple initializations concurrently
	return tea.Batch(
		// Start toast cleanup ticker
		m.startToastCleanupTicker(),
		// Pre-load context if available
		m.loadContextCmd(),
		// Query daemon for recent history
		m.queryDaemonHistoryCmd(),
		// Query daemon status and start periodic checks
		m.queryDaemonStatusCmd(),
		m.startDaemonStatusTicker(),
	)
}

// startToastCleanupTicker returns a command to periodically clean expired toasts
func (m Model) startToastCleanupTicker() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return toastCleanupTickMsg{Time: t}
	})
}

// loadContextCmd returns a command to load context asynchronously
func (m Model) loadContextCmd() tea.Cmd {
	return func() tea.Msg {
		return contextLoadedMsg{}
	}
}

// queryDaemonHistoryCmd queries the daemon for edit history for current workspace
func (m Model) queryDaemonHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		// Get current workspace path
		workspacePath, err := os.Getwd()
		if err != nil {
			logger.Log("Failed to get working directory: %v", err)
			return daemonHistoryMsg{err: err}
		}

		// Try to connect to daemon query socket
		querySocket := "/tmp/claude-mon-query.sock"
		conn, err := net.DialTimeout("unix", querySocket, 2*time.Second)
		if err != nil {
			logger.Log("Daemon not available: %v", err)
			return daemonHistoryMsg{err: err}
		}
		defer conn.Close()

		// Set read/write deadline
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		// Send query for edits in this workspace
		query := map[string]interface{}{
			"type":           "workspace",
			"workspace_path": workspacePath,
			"limit":          100,
		}
		if err := json.NewEncoder(conn).Encode(query); err != nil {
			logger.Log("Failed to send query: %v", err)
			return daemonHistoryMsg{err: err}
		}

		// Read response
		var result struct {
			Type  string `json:"type"`
			Edits []struct {
				ID          int64     `json:"id"`
				SessionID   int64     `json:"session_id"`
				ToolName    string    `json:"tool_name"`
				FilePath    string    `json:"file_path"`
				OldString   string    `json:"old_string"`
				NewString   string    `json:"new_string"`
				LineNum     int       `json:"line_num"`
				LineCount   int       `json:"line_count"`
				CommitSHA   string    `json:"commit_sha"`
				VCSType     string    `json:"vcs_type"`
				FileContent string    `json:"file_content"`
				CreatedAt   time.Time `json:"created_at"`
			} `json:"edits"`
			Error string `json:"error,omitempty"`
		}

		if err := json.NewDecoder(conn).Decode(&result); err != nil {
			logger.Log("Failed to decode response: %v", err)
			return daemonHistoryMsg{err: err}
		}

		if result.Error != "" {
			logger.Log("Daemon error: %s", result.Error)
			return daemonHistoryMsg{err: fmt.Errorf("daemon: %s", result.Error)}
		}

		// Convert edits to changes
		var changes []Change
		for _, edit := range result.Edits {
			change := Change{
				Timestamp:   edit.CreatedAt,
				FilePath:    edit.FilePath,
				ToolName:    edit.ToolName,
				OldString:   edit.OldString,
				NewString:   edit.NewString,
				LineNum:     edit.LineNum,
				LineCount:   edit.LineCount,
				CommitSHA:   edit.CommitSHA,
				VCSType:     edit.VCSType,
				FileContent: edit.FileContent,
			}
			// Set short commit SHA for display
			if len(edit.CommitSHA) >= 8 {
				change.CommitShort = edit.CommitSHA[:8]
			} else if edit.CommitSHA != "" {
				change.CommitShort = edit.CommitSHA
			}
			changes = append(changes, change)
		}

		logger.Log("Loaded %d edits from daemon", len(changes))
		return daemonHistoryMsg{changes: changes}
	}
}

// queryDaemonStatusCmd queries the daemon for its status and workspace activity
func (m Model) queryDaemonStatusCmd() tea.Cmd {
	return func() tea.Msg {
		// Get current workspace path
		workspacePath, err := os.Getwd()
		if err != nil {
			logger.Log("Failed to get working directory: %v", err)
			return daemonStatusMsg{connected: false}
		}

		// Try to connect to daemon query socket
		querySocket := "/tmp/claude-mon-query.sock"
		conn, err := net.DialTimeout("unix", querySocket, 1*time.Second)
		if err != nil {
			// Daemon not running - not an error, just mark as disconnected
			return daemonStatusMsg{connected: false}
		}
		defer conn.Close()

		// Set read/write deadline
		conn.SetDeadline(time.Now().Add(2 * time.Second))

		// Send status query for this workspace
		query := map[string]interface{}{
			"type":           "status",
			"workspace_path": workspacePath,
		}
		if err := json.NewEncoder(conn).Encode(query); err != nil {
			logger.Log("Failed to send status query: %v", err)
			return daemonStatusMsg{connected: false}
		}

		// Read response
		var result struct {
			Type   string `json:"type"`
			Status struct {
				Running   bool   `json:"running"`
				UptimeStr string `json:"uptime_str"`
				Active    *struct {
					Path         string    `json:"path"`
					Name         string    `json:"name"`
					LastActivity time.Time `json:"last_activity"`
					EditCount    int       `json:"edit_count"`
				} `json:"active_workspace,omitempty"`
			} `json:"status"`
			Error string `json:"error,omitempty"`
		}

		if err := json.NewDecoder(conn).Decode(&result); err != nil {
			logger.Log("Failed to decode status response: %v", err)
			return daemonStatusMsg{connected: false}
		}

		if result.Error != "" {
			logger.Log("Daemon status error: %s", result.Error)
			return daemonStatusMsg{connected: false}
		}

		msg := daemonStatusMsg{
			connected: true,
			uptime:    result.Status.UptimeStr,
		}

		if result.Status.Active != nil {
			msg.workspaceActive = true
			msg.workspaceEdits = result.Status.Active.EditCount
			msg.lastActivity = result.Status.Active.LastActivity
		}

		return msg
	}
}

// startDaemonStatusTicker returns a command that starts the daemon status check ticker
func (m Model) startDaemonStatusTicker() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return daemonStatusTickMsg{t}
	})
}

// LeaderActivatedAt returns when leader mode was activated
func (m Model) LeaderActivatedAt() time.Time {
	return m.leaderActivatedAt
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

		// Update help width for bubbles/help
		m.help.Width = msg.Width

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

		key := msg.String()

		// Handle leader key mode
		if m.leaderActive {
			return m.handleLeaderKey(msg)
		}

		// Activate leader key mode (ctrl+g by default)
		if key == m.config.LeaderKey {
			logger.Log("Leader mode activated")
			m.leaderActive = true
			m.leaderActivatedAt = time.Now()
			// Start timeout - auto-dismiss after 4 seconds
			return m, tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
				return leaderTimeoutMsg{activatedAt: m.leaderActivatedAt}
			})
		}

		// Handle plan input mode - must check BEFORE global keys
		if m.planInputActive {
			switch key {
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
		}

		// Handle context edit mode - must check BEFORE global keys
		if m.contextEditMode {
			switch key {
			case "enter":
				// If completion overlay is active, select the completion
				if m.contextCompletionActive {
					if len(m.contextCompletionMatches) > 0 && m.contextCompletionSelected < len(m.contextCompletionMatches) {
						idx := m.contextCompletionMatches[m.contextCompletionSelected]
						selected := m.contextCompletionCandidates[idx]
						m.setCurrentContextFieldValue(selected)
					}
					m.contextCompletionActive = false
					m.contextCompletionInput.Reset()
					m.contextCompletionInput.Blur()
					return m, nil
				}
				// Save the edited value based on context type
				m.saveContextEdit()
				m.contextEditMode = false
				return m, nil
			case "esc":
				// If completion is active, close it first
				if m.contextCompletionActive {
					m.contextCompletionActive = false
					m.contextCompletionInput.Reset()
					m.contextCompletionInput.Blur()
					return m, nil
				}
				// Cancel editing
				m.contextEditMode = false
				m.contextEditField = ""
				return m, nil
			case "tab":
				// Move to next field or toggle completion
				if m.contextCompletionActive {
					m.contextCompletionActive = false
					m.contextCompletionInput.Reset()
					m.contextCompletionInput.Blur()
				} else {
					// Move to next field
					m.nextContextField()
				}
				return m, nil
			case "shift+tab":
				// Move to previous field
				m.prevContextField()
				return m, nil
			case "ctrl+@":
				// Open completion for current field (ctrl+space)
				if !m.contextCompletionActive {
					m.loadContextCompletions()
					m.contextCompletionActive = true
					m.contextCompletionInput.Reset()
					m.contextCompletionInput.Focus()
				}
				return m, nil
			default:
				// If completion overlay is active, handle its keys
				if m.contextCompletionActive {
					switch key {
					case "up", "ctrl+p":
						if m.contextCompletionSelected > 0 {
							m.contextCompletionSelected--
						}
						return m, nil
					case "down", "ctrl+n":
						if m.contextCompletionSelected < len(m.contextCompletionMatches)-1 {
							m.contextCompletionSelected++
						}
						return m, nil
					default:
						// Forward to completion filter input
						var cmd tea.Cmd
						m.contextCompletionInput, cmd = m.contextCompletionInput.Update(msg)
						m.computeContextCompletionMatches(m.contextCompletionInput.Value())
						if m.contextCompletionSelected >= len(m.contextCompletionMatches) {
							m.contextCompletionSelected = 0
						}
						return m, cmd
					}
				}
				// Forward to current focused input
				return m.updateCurrentContextInput(msg)
			}
		}

		// Global keys (work in any mode)
		switch key {
		case m.config.Keys.Help:
			m.showHelp = true
			return m, nil
		case m.config.Keys.NextTab:
			// Cycle to next tab/mode
			m.cycleMode(1)
			return m, nil
		case m.config.Keys.PrevTab:
			// Cycle to previous tab/mode
			m.cycleMode(-1)
			return m, nil
		case m.config.Keys.LeftPane:
			// Switch to left pane (only if visible)
			if !m.hideLeftPane {
				m.activePane = PaneLeft
			}
			return m, nil
		case m.config.Keys.RightPane:
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
			return m, m.ralphRefreshCmd
		case "4":
			// Direct access to Plan tab
			m.switchToMode(LeftPaneModePlan)
			return m, nil
		case "5":
			// Direct access to Context tab
			m.switchToMode(LeftPaneModeContext)
			return m, nil
		case m.config.Keys.ToggleMinimap:
			m.showMinimap = !m.showMinimap
			m.updateViewportSize()
			m.diffViewport.SetContent(m.renderRightPane())
			return m, nil
		case m.config.Keys.ToggleLeftPane:
			m.hideLeftPane = !m.hideLeftPane
			// Force right pane focus when left pane is hidden
			if m.hideLeftPane {
				m.activePane = PaneRight
			}
			m.updateViewportSize()
			m.diffViewport.SetContent(m.renderRightPane())
			return m, nil
		case m.config.Keys.Quit:
			return m, tea.Quit
		}

		// Mode-specific key handling
		switch m.leftPaneMode {
		case LeftPaneModePrompts:
			return m.handlePromptsKeys(msg)
		case LeftPaneModeRalph:
			return m.handleRalphKeys(msg)
		case LeftPaneModePlan:
			return m.handlePlanKeys(msg)
		case LeftPaneModeContext:
			return m.handleContextKeys(msg)
		default:
			return m.handleHistoryKeys(msg)
		}

	case SocketMsg:
		logger.Log("SocketMsg received, payload size: %d bytes", len(msg.Payload))
		m.lastMsgTime = time.Now() // Track last message for status indicator

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

			// Select the newly added change (most recent, at top of visual list)
			m.selectedIndex = len(m.changes) - 1
			m.scrollX = 0
			m.listScrollOffset = 0 // Keep newest visible at top
			m.ensureSelectedVisible()
			m.diffViewport.SetContent(m.renderDiff())
		} else {
			logger.Log("parsePayload returned nil")
		}

	case promptEditedMsg:
		// Prompt was edited in nvim - update frontmatter and refresh list
		logger.Log("Prompt edited: %s, leftPaneMode=%d", msg.path, m.leftPaneMode)
		m.leftPaneMode = LeftPaneModePrompts // Ensure we stay in prompts mode

		// Update version and timestamp in frontmatter
		if m.promptStore != nil {
			if err := m.promptStore.UpdateAfterEdit(msg.path); err != nil {
				logger.Log("Failed to update prompt frontmatter: %v", err)
			}
		}

		m.refreshPromptList()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Prompt saved", ToastSuccess)

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

	case planEditedMsg:
		logger.Log("Plan edited, reloading")
		m.loadPlanFile()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Plan reloaded", ToastInfo)

	case leaderTimeoutMsg:
		// Only dismiss if this timeout matches current activation
		if m.leaderActive && msg.activatedAt.Equal(m.leaderActivatedAt) {
			logger.Log("Leader mode timed out")
			m.leaderActive = false
		}

	case ralphRefreshTickMsg:
		// Auto-refresh Ralph state when in Ralph mode
		if m.leftPaneMode == LeftPaneModeRalph {
			logger.Log("Auto-refreshing Ralph state")
			m.loadRalphState()
			// Return the command again to keep the ticker going
			return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
				return ralphRefreshTickMsg{Time: t}
			})
		}

	case toastCleanupTickMsg:
		// Clean expired toasts and keep ticker running
		m.cleanExpiredToasts()
		return m, m.startToastCleanupTicker()

	case contextLoadedMsg:
		// Context loaded - nothing to do, already handled in New()

	case daemonHistoryMsg:
		if msg.err != nil {
			// Daemon not available - that's OK, we can still receive live updates
			logger.Log("Daemon query failed (will use live updates): %v", msg.err)
		} else if len(msg.changes) > 0 {
			// Only add changes we don't already have (avoid duplicates with local history)
			existingPaths := make(map[string]bool)
			for _, c := range m.changes {
				key := fmt.Sprintf("%s:%s:%d", c.FilePath, c.Timestamp.Format(time.RFC3339), c.LineNum)
				existingPaths[key] = true
			}

			for _, c := range msg.changes {
				key := fmt.Sprintf("%s:%s:%d", c.FilePath, c.Timestamp.Format(time.RFC3339), c.LineNum)
				if !existingPaths[key] {
					m.changes = append(m.changes, c)
				}
			}

			// Select most recent (newest is at highest index)
			if len(m.changes) > 0 {
				m.selectedIndex = len(m.changes) - 1
				m.listScrollOffset = 0 // Start at top showing newest
				m.ensureSelectedVisible()
				m.diffViewport.SetContent(m.renderDiff())
			}
			m.lastMsgTime = time.Now()
			logger.Log("Added %d changes from daemon, total now: %d", len(msg.changes), len(m.changes))
		}

	case daemonStatusMsg:
		m.daemonConnected = msg.connected
		m.daemonUptime = msg.uptime
		m.daemonLastCheck = time.Now()
		m.daemonWorkspaceActive = msg.workspaceActive
		m.daemonWorkspaceEdits = msg.workspaceEdits
		m.daemonLastActivity = msg.lastActivity

	case daemonStatusTickMsg:
		// Periodic daemon status check
		cmds = append(cmds, m.queryDaemonStatusCmd(), m.startDaemonStatusTicker())
	}

	return m, tea.Batch(cmds...)
}

// handleHistoryKeys handles key events in history mode
func (m Model) handleHistoryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case m.config.Keys.Down, "down":
		if m.activePane == PaneLeft {
			// Navigate history list down (older items = lower index)
			// Data is oldest-first, display is newest-first (reversed)
			if len(m.changes) > 0 && m.selectedIndex > 0 {
				m.selectedIndex--
				m.scrollX = 0
				m.ensureSelectedVisible()
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.diffViewport.LineDown(1)
		}
	case m.config.Keys.Up, "up":
		if m.activePane == PaneLeft {
			// Navigate history list up (newer items = higher index)
			if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
				m.selectedIndex++
				m.scrollX = 0
				m.ensureSelectedVisible()
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.diffViewport.LineUp(1)
		}
	case m.config.Keys.PageDown:
		if m.activePane == PaneLeft {
			// Page down in history list (older items = lower indices)
			visibleItems := m.listVisibleItems()
			for i := 0; i < visibleItems && m.selectedIndex > 0; i++ {
				m.selectedIndex--
			}
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		} else {
			m.diffViewport.ViewDown()
		}
	case m.config.Keys.PageUp:
		if m.activePane == PaneLeft {
			// Page up in history list (newer items = higher indices)
			visibleItems := m.listVisibleItems()
			for i := 0; i < visibleItems && m.selectedIndex < len(m.changes)-1; i++ {
				m.selectedIndex++
			}
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		} else {
			m.diffViewport.ViewUp()
		}
	case m.config.Keys.Next:
		// Next change in time (older = lower index)
		if len(m.changes) > 0 && m.selectedIndex > 0 {
			m.selectedIndex--
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}
	case m.config.Keys.Prev:
		// Previous change in time (newer = higher index)
		if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
			m.selectedIndex++
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.diffViewport.SetContent(m.renderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}
	case m.config.Keys.ScrollLeft:
		if m.scrollX > 0 {
			m.scrollX -= 4
			if m.scrollX < 0 {
				m.scrollX = 0
			}
			m.diffViewport.SetContent(m.renderDiff())
		}
	case m.config.Keys.ScrollRight:
		m.scrollX += 4
		m.diffViewport.SetContent(m.renderDiff())
	case m.config.Keys.ClearHistory:
		m.changes = []Change{}
		m.selectedIndex = 0
		m.listScrollOffset = 0
		m.diffViewport.SetContent("")
		m.diffCache = make(map[int]string)
		if m.persistHistory && m.historyStore != nil {
			if err := m.historyStore.Clear(); err != nil {
				logger.Log("Failed to clear history file: %v", err)
			}
		}
	case m.config.Keys.OpenInNvim:
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	case m.config.Keys.OpenNvimCwd:
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
	key := msg.String()

	// Fuzzy filter mode has different key bindings
	if m.promptFuzzyActive {
		switch key {
		case "esc":
			// Cancel fuzzy filter
			m.promptFuzzyActive = false
			m.promptFuzzyInput.Reset()
			m.promptFuzzyInput.Blur()
			return m, nil
		case "enter":
			// Select the fuzzy match
			if len(m.promptFuzzyMatches) > 0 && m.promptFuzzySelected < len(m.promptFuzzyMatches) {
				m.promptSelected = m.promptFuzzyMatches[m.promptFuzzySelected]
				m.promptFuzzyActive = false
				m.promptFuzzyInput.Reset()
				m.promptFuzzyInput.Blur()
				m.diffViewport.SetContent(m.renderRightPane())
			}
			return m, nil
		case "up", "ctrl+p":
			// Navigate up in fuzzy matches
			if m.promptFuzzySelected > 0 {
				m.promptFuzzySelected--
			}
			return m, nil
		case "down", "ctrl+n":
			// Navigate down in fuzzy matches
			if m.promptFuzzySelected < len(m.promptFuzzyMatches)-1 {
				m.promptFuzzySelected++
			}
			return m, nil
		default:
			// Pass to text input for typing
			var cmd tea.Cmd
			m.promptFuzzyInput, cmd = m.promptFuzzyInput.Update(msg)
			// Recompute matches on every keystroke
			m.promptFuzzyMatches = m.computeFuzzyMatches(m.promptFuzzyInput.Value())
			// Reset selection if it's out of bounds
			if m.promptFuzzySelected >= len(m.promptFuzzyMatches) {
				m.promptFuzzySelected = 0
			}
			return m, cmd
		}
	}

	// Version view mode has different key bindings
	if m.promptShowVersions {
		switch key {
		case m.config.Keys.ViewVersions, "shift+v", "esc":
			// Exit version view, back to prompt list
			m.promptShowVersions = false
			m.promptVersionSelected = 0
			m.diffViewport.SetContent(m.renderRightPane())
		case m.config.Keys.Down, "down":
			if m.promptVersionSelected < len(m.promptVersions)-1 {
				m.promptVersionSelected++
				m.diffViewport.SetContent(m.renderRightPane())
			}
		case m.config.Keys.Up, "up":
			if m.promptVersionSelected > 0 {
				m.promptVersionSelected--
				m.diffViewport.SetContent(m.renderRightPane())
			}
		case m.config.Keys.RevertVersion, m.config.Keys.SendPrompt:
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
		case m.config.Keys.DeletePrompt:
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
		case m.config.Keys.EditPrompt:
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
	switch key {
	case m.config.Keys.Down, "down":
		if m.activePane == PaneLeft && m.promptSelected < len(m.promptFilteredList)-1 {
			m.promptSelected++
			m.diffViewport.SetContent(m.renderRightPane())
		} else if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case m.config.Keys.Up, "up":
		if m.activePane == PaneLeft && m.promptSelected > 0 {
			m.promptSelected--
			m.diffViewport.SetContent(m.renderRightPane())
		} else if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case m.config.Keys.NewPrompt:
		// New project-local prompt - open nvim with template
		return m.createNewPrompt(false)
	case m.config.Keys.NewGlobalPrompt:
		// New global prompt - open nvim with template
		return m.createNewPrompt(true)
	case m.config.Keys.EditPrompt:
		// Edit selected prompt
		if len(m.promptFilteredList) > 0 {
			return m.editPrompt(m.promptFilteredList[m.promptSelected])
		}
	case m.config.Keys.CreateVersion:
		// Create version backup
		logger.Log("Version key pressed: promptFilteredList=%d, promptStore=%v", len(m.promptFilteredList), m.promptStore != nil)
		if len(m.promptFilteredList) > 0 && m.promptStore != nil {
			p := m.promptFilteredList[m.promptSelected]
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
	case m.config.Keys.ViewVersions, "shift+v":
		// Enter version view mode
		if len(m.promptFilteredList) > 0 && m.promptStore != nil {
			m.loadVersionList()
			if len(m.promptVersions) > 0 {
				m.promptShowVersions = true
				m.promptVersionSelected = 0
				m.diffViewport.SetContent(m.renderRightPane())
			} else {
				m.addToast("No versions found", ToastWarning)
			}
		}
	case m.config.Keys.DeletePrompt:
		// Delete prompt
		if len(m.promptFilteredList) > 0 && m.promptStore != nil {
			p := m.promptFilteredList[m.promptSelected]
			if err := m.promptStore.Delete(p.Path); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast("Deleted "+p.Name, ToastSuccess)
				m.refreshPromptList()
				m.diffViewport.SetContent(m.renderRightPane())
			}
		}
	case m.config.Keys.SendPrompt:
		// Inject prompt using current method
		if len(m.promptFilteredList) > 0 {
			p := m.promptFilteredList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			logger.Log("Injecting prompt: original=%d bytes, expanded=%d bytes", len(p.Content), len(expanded))
			if err := prompt.Inject(expanded, m.promptInjectMethod); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast(fmt.Sprintf("Sent via %s", prompt.MethodName(m.promptInjectMethod)), ToastSuccess)
			}
		}
	case m.config.Keys.YankPrompt:
		// Yank/copy to clipboard only
		if len(m.promptFilteredList) > 0 {
			p := m.promptFilteredList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			if err := prompt.Inject(expanded, prompt.InjectClipboard); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast("Copied to clipboard", ToastSuccess)
			}
		}
	case m.config.Keys.InjectMethod:
		// Cycle injection method
		m.promptInjectMethod = (m.promptInjectMethod + 1) % 2
		m.addToast(fmt.Sprintf("Inject method: %s", prompt.MethodName(m.promptInjectMethod)), ToastInfo)
	case "/":
		// Cycle filter scope: all -> project -> global -> all
		m.promptFilter = (m.promptFilter + 1) % 3
		m.applyPromptFilter()
		var scopeName string
		switch m.promptFilter {
		case PromptFilterAll:
			scopeName = "All"
		case PromptFilterProject:
			scopeName = "Project"
		case PromptFilterGlobal:
			scopeName = "Global"
		}
		m.addToast(fmt.Sprintf("Filter: %s", scopeName), ToastInfo)
		m.diffViewport.SetContent(m.renderRightPane())
	case "f":
		// Activate fuzzy filter overlay
		if len(m.promptFilteredList) > 0 {
			m.promptFuzzyActive = true
			m.promptFuzzyInput.Reset()
			m.promptFuzzyInput.Focus()
			m.promptFuzzyMatches = m.computeFuzzyMatches("")
			m.promptFuzzySelected = 0
		}
	}
	return m, nil
}

// handleRalphKeys handles key events in Ralph mode
func (m Model) handleRalphKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case m.config.Keys.Down, "down":
		if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case m.config.Keys.Up, "up":
		if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case m.config.Keys.CancelRalph:
		// Cancel Ralph loop
		if m.ralphState != nil && m.ralphState.Active {
			if removed, _ := ralph.CancelLoop(); removed {
				m.ralphState = nil
				m.addToast("Ralph Loop cancelled", ToastSuccess)
				m.diffViewport.SetContent(m.renderRightPane())
			}
		}
	case m.config.Keys.Refresh:
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
	case m.config.Keys.Down, "down":
		if m.activePane == PaneRight {
			m.diffViewport.LineDown(1)
		}
	case m.config.Keys.Up, "up":
		if m.activePane == PaneRight {
			m.diffViewport.LineUp(1)
		}
	case m.config.Keys.PageDown:
		if m.activePane == PaneRight {
			m.diffViewport.HalfViewDown()
		}
	case m.config.Keys.PageUp:
		if m.activePane == PaneRight {
			m.diffViewport.HalfViewUp()
		}
	case m.config.Keys.GeneratePlan:
		// Generate new plan
		if !m.planGenerating {
			m.planInputActive = true
			m.planInput.Focus()
			return m, textinput.Blink
		}
	case m.config.Keys.EditPlan:
		// Edit plan in nvim
		if m.planPath != "" {
			cmd := exec.Command("nvim", m.planPath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	case m.config.Keys.Refresh:
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

// handleContextKeys handles key events in Context mode
// All context actions are now behind leader key, so this only handles scrolling
func (m Model) handleContextKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle scrolling in right pane
	if m.activePane == PaneRight {
		switch key {
		case m.config.Keys.Down, "down":
			m.diffViewport.LineDown(1)
		case m.config.Keys.Up, "up":
			m.diffViewport.LineUp(1)
		case m.config.Keys.PageDown:
			m.diffViewport.HalfViewDown()
		case m.config.Keys.PageUp:
			m.diffViewport.HalfViewUp()
		}
	}

	return m, nil
}

// handleLeaderKey handles context-sensitive key actions when leader mode is active
func (m Model) handleLeaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Escape cancels leader mode
	if key == "esc" {
		m.leaderActive = false
		return m, nil
	}

	// Exit leader mode immediately when any action key is pressed
	m.leaderActive = false

	// Global actions (available in any context)
	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "h":
		m.hideLeftPane = !m.hideLeftPane
		if m.hideLeftPane {
			m.activePane = PaneRight
		}
		m.updateViewportSize()
		m.diffViewport.SetContent(m.renderRightPane())
		return m, nil
	case "m":
		m.showMinimap = !m.showMinimap
		m.updateViewportSize()
		m.diffViewport.SetContent(m.renderRightPane())
		return m, nil
	case "1":
		m.switchToMode(LeftPaneModeHistory)
		return m, nil
	case "2":
		m.switchToMode(LeftPaneModePrompts)
		return m, nil
	case "3":
		m.switchToMode(LeftPaneModeRalph)
		return m, m.ralphRefreshCmd
	case "4":
		m.switchToMode(LeftPaneModePlan)
		return m, nil
	case "5":
		m.switchToMode(LeftPaneModeContext)
		return m, nil
	}

	// Context-sensitive actions based on pane and mode
	if m.activePane == PaneRight {
		return m.handleLeaderKeyRightPane(key)
	}

	// Left pane - mode-specific
	switch m.leftPaneMode {
	case LeftPaneModeHistory:
		return m.handleLeaderKeyHistory(key)
	case LeftPaneModePrompts:
		return m.handleLeaderKeyPrompts(key)
	case LeftPaneModeRalph:
		return m.handleLeaderKeyRalph(key)
	case LeftPaneModePlan:
		return m.handleLeaderKeyPlan(key)
	case LeftPaneModeContext:
		return m.handleLeaderKeyContext(key)
	}

	return m, nil
}

// handleLeaderKeyRightPane handles leader keys when right pane is focused
func (m Model) handleLeaderKeyRightPane(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "g": // Open in nvim at line
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return nil })
		}
	case "o": // Open in nvim (file only)
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return nil })
		}
	}
	return m, nil
}

// handleLeaderKeyHistory handles leader keys in history mode
func (m Model) handleLeaderKeyHistory(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "g": // Open in nvim at line
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return nil })
		}
	case "o": // Open in nvim (file only)
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return nil })
		}
	case "x": // Clear history
		m.changes = nil
		m.selectedIndex = 0
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("History cleared", ToastInfo)
	}
	return m, nil
}

// handleLeaderKeyPrompts handles leader keys in prompts mode
func (m Model) handleLeaderKeyPrompts(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "n": // New prompt
		return m.createNewPrompt(false)
	case "N": // New global prompt
		return m.createNewPrompt(true)
	case "e": // Edit prompt
		if len(m.promptList) > 0 {
			return m.editPrompt(m.promptList[m.promptSelected])
		}
	case "y": // Yank prompt
		if len(m.promptList) > 0 {
			p := m.promptList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			if err := prompt.Inject(expanded, prompt.InjectClipboard); err != nil {
				m.addToast("Failed to copy", ToastError)
			} else {
				m.addToast("Copied to clipboard", ToastSuccess)
			}
		}
	case "d": // Delete prompt
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
	case "v": // Create version
		if len(m.promptList) > 0 && m.promptStore != nil {
			p := m.promptList[m.promptSelected]
			if err := m.promptStore.CreateVersion(&p); err != nil {
				m.addToast(err.Error(), ToastError)
			} else {
				m.addToast(fmt.Sprintf("Created v%d backup", p.Version), ToastSuccess)
				m.refreshPromptList()
				m.diffViewport.SetContent(m.renderRightPane())
			}
		}
	case "V": // View versions
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
	case "i": // Cycle inject method
		m.promptInjectMethod = (m.promptInjectMethod + 1) % 2
		m.addToast(fmt.Sprintf("Method: %s", prompt.MethodName(m.promptInjectMethod)), ToastInfo)
	case "enter": // Send prompt (via inject method)
		if len(m.promptList) > 0 {
			p := m.promptList[m.promptSelected]
			expanded := m.expandPromptVariables(p.Content)
			if err := prompt.Inject(expanded, m.promptInjectMethod); err != nil {
				m.addToast("Failed to inject", ToastError)
			} else {
				m.addToast(fmt.Sprintf("Sent via %s", prompt.MethodName(m.promptInjectMethod)), ToastSuccess)
			}
		}
	}
	return m, nil
}

// handleLeaderKeyRalph handles leader keys in ralph mode
func (m Model) handleLeaderKeyRalph(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "C": // Cancel ralph
		if _, err := ralph.CancelLoop(); err != nil {
			m.addToast(err.Error(), ToastError)
		} else {
			m.addToast("Ralph cancelled", ToastSuccess)
			m.loadRalphState()
		}
	case "r": // Refresh
		m.loadRalphState()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Refreshed", ToastInfo)
	}
	return m, nil
}

// handleLeaderKeyPlan handles leader keys in plan mode
func (m Model) handleLeaderKeyPlan(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "G": // Generate plan - activate input mode
		m.planInputActive = true
		m.planInput.Focus()
		m.addToast("Enter plan description", ToastInfo)
	case "e": // Edit plan
		if m.planPath != "" {
			cmd := exec.Command("nvim", m.planPath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return planEditedMsg{}
			})
		}
	case "r": // Refresh
		m.loadPlanFile()
		m.diffViewport.SetContent(m.renderRightPane())
		m.addToast("Refreshed", ToastInfo)
	}
	return m, nil
}

// handleLeaderKeyContext handles leader keys in context mode
func (m Model) handleLeaderKeyContext(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "k":
		// Set Kubernetes context - multi-field: kubeconfig, context, namespace
		m.contextEditMode = true
		m.contextEditField = "k8s"
		m.k8sFocusedField = 0 // Start at kubeconfig
		// Pre-fill from current context
		if k8s := m.contextCurrent.GetKubernetes(); k8s != nil {
			m.k8sKubeconfigInput.SetValue(k8s.Kubeconfig)
			m.k8sContextInput.SetValue(k8s.Context)
			m.k8sNamespaceInput.SetValue(k8s.Namespace)
		} else {
			m.k8sKubeconfigInput.Reset()
			m.k8sContextInput.Reset()
			m.k8sNamespaceInput.Reset()
		}
		m.k8sKubeconfigInput.Focus()
		m.k8sContextInput.Blur()
		m.k8sNamespaceInput.Blur()
		return m, textinput.Blink
	case "a":
		// Set AWS profile - multi-field: profile, region
		m.contextEditMode = true
		m.contextEditField = "aws"
		m.awsFocusedField = 0 // Start at profile
		// Pre-fill from current context
		if aws := m.contextCurrent.GetAWS(); aws != nil {
			m.awsProfileInput.SetValue(aws.Profile)
			m.awsRegionInput.SetValue(aws.Region)
		} else {
			m.awsProfileInput.Reset()
			m.awsRegionInput.Reset()
		}
		m.awsProfileInput.Focus()
		m.awsRegionInput.Blur()
		return m, textinput.Blink
	case "g":
		// Set Git info - multi-field: branch, repo
		m.contextEditMode = true
		m.contextEditField = "git"
		m.gitFocusedField = 0 // Start at branch
		// Pre-fill from current context
		if git := m.contextCurrent.GetGit(); git != nil {
			m.gitBranchInput.SetValue(git.Branch)
			m.gitRepoInput.SetValue(git.Repo)
		} else {
			m.gitBranchInput.Reset()
			m.gitRepoInput.Reset()
		}
		m.gitBranchInput.Focus()
		m.gitRepoInput.Blur()
		return m, textinput.Blink
	case "e":
		// Set environment variables - single KEY=VALUE field
		m.contextEditMode = true
		m.contextEditField = "env"
		m.envInput.Reset()
		m.envInput.Focus()
		return m, textinput.Blink
	case "c":
		// Set custom values - single KEY=VALUE field
		m.contextEditMode = true
		m.contextEditField = "custom"
		m.customInput.Reset()
		m.customInput.Focus()
		return m, textinput.Blink
	case "K":
		// Clear Kubernetes context
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("kubernetes")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear k8s: %v", err), ToastError)
			} else {
				m.addToast("Kubernetes context cleared", ToastSuccess)
			}
		}
	case "A":
		// Clear AWS context
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("aws")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear AWS: %v", err), ToastError)
			} else {
				m.addToast("AWS context cleared", ToastSuccess)
			}
		}
	case "G":
		// Clear Git context
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("git")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear Git: %v", err), ToastError)
			} else {
				m.addToast("Git context cleared", ToastSuccess)
			}
		}
	case "E":
		// Clear environment variables
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("env")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear env: %v", err), ToastError)
			} else {
				m.addToast("Environment variables cleared", ToastSuccess)
			}
		}
	case "X":
		// Clear custom values
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("custom")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear custom: %v", err), ToastError)
			} else {
				m.addToast("Custom values cleared", ToastSuccess)
			}
		}
	case "C":
		// Clear all context
		if m.contextCurrent != nil {
			m.contextCurrent.Clear("all")
			if err := m.contextCurrent.Save(); err != nil {
				m.addToast(fmt.Sprintf("Failed to clear context: %v", err), ToastError)
			} else {
				// Also clear via CLI
				cmd := exec.Command("claude", "-p", "/prompt:context clear", "--mcp", "{}")
				cmd.Env = append(os.Environ(), "CLAUDE_CODE_ENTRYPOINT=cli")
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						logger.Log("Failed to clear context via CLI: %v", err)
					}
					return nil
				})
			}
		}
	case "r":
		// Reload context from disk
		if ctx, err := workingctx.Load(); err == nil {
			m.contextCurrent = ctx
			m.addToast("Context reloaded", ToastSuccess)
		} else {
			m.addToast(fmt.Sprintf("Failed to reload context: %v", err), ToastError)
		}
	case "l":
		// Toggle showing all contexts list
		m.contextShowList = !m.contextShowList
		if m.contextShowList {
			m.addToast("Showing all contexts", ToastInfo)
		} else {
			m.addToast("Hiding context list", ToastInfo)
		}
	}
	return m, nil
}

// cycleMode cycles through the available modes
func (m *Model) cycleMode(direction int) {
	modes := []LeftPaneMode{LeftPaneModeHistory, LeftPaneModePrompts, LeftPaneModeRalph, LeftPaneModePlan, LeftPaneModeContext}
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
	logger.Log("switchToMode: %d -> %d", prevMode, mode)
	m.leftPaneMode = mode
	m.activePane = PaneLeft
	m.promptShowVersions = false

	// Cancel Ralph refresh ticker when leaving Ralph mode
	if prevMode == LeftPaneModeRalph && mode != LeftPaneModeRalph {
		if m.ralphRefreshCmd != nil {
			m.ralphRefreshCmd = nil
			logger.Log("Cancelled Ralph refresh ticker")
		}
	}

	// Mode-specific initialization
	switch mode {
	case LeftPaneModePrompts:
		m.refreshPromptList()
	case LeftPaneModeRalph:
		m.loadRalphState()
		// Start auto-refresh ticker (every 5 seconds)
		m.ralphRefreshCmd = tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return ralphRefreshTickMsg{Time: t}
		})
		logger.Log("Started Ralph refresh ticker (5s interval)")
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

// renderTabBar renders the tab bar with all 5 modes
func (m Model) renderTabBar() string {
	tabs := []struct {
		num  string
		name string
		mode LeftPaneMode
		icon string
	}{
		{"1", "History", LeftPaneModeHistory, "📜"},
		{"2", "Prompts", LeftPaneModePrompts, "📝"},
		{"3", "Ralph", LeftPaneModeRalph, "🔄"},
		{"4", "Plan", LeftPaneModePlan, "📋"},
		{"5", "Context", LeftPaneModeContext, "⚙️"},
	}

	var parts []string
	for _, tab := range tabs {
		if tab.mode == m.leftPaneMode {
			// Active tab - show full name, highlighted
			label := tab.num + ":" + tab.name
			parts = append(parts, m.theme.Selected.Render("["+label+"]"))
		} else {
			// Inactive tab - show icon only
			label := tab.num + ":" + tab.icon

			// Add state indicator for active states
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

// renderContextList renders the context management view for the full-width pane
func (m Model) renderContextList() string {
	var sb strings.Builder

	// Reload context to ensure we have latest data
	if m.contextCurrent == nil {
		if ctx, err := workingctx.Load(); err == nil {
			m.contextCurrent = ctx
		}
	}

	// Title
	sb.WriteString(m.theme.Title.Render("⚙️ Working Context\n\n"))

	if m.contextCurrent == nil {
		sb.WriteString(m.theme.Dim.Render("No context available"))
		return sb.String()
	}

	// Project info
	sb.WriteString(m.theme.Selected.Render("📁 Project:") + " ")
	sb.WriteString(m.theme.Normal.Render(m.contextCurrent.ProjectRoot) + "\n\n")

	// Show current context
	ctx := m.contextCurrent.Format()
	lines := strings.Split(ctx, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Project:") {
			// Skip project line as we already showed it
			continue
		}
		if line == "" {
			sb.WriteString("\n")
			continue
		}
		// Format key-value pairs
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				sb.WriteString(m.theme.Dim.Render(key+": ") + m.theme.Normal.Render(value) + "\n")
			}
		} else {
			sb.WriteString(line + "\n")
		}
	}

	// Stale warning
	if m.contextCurrent.IsStale() {
		sb.WriteString("\n")
		sb.WriteString(m.theme.Status.Render("⚠️ Context is stale (>24h)"))
		sb.WriteString("\n")
	}

	// Show list of all contexts if requested
	if m.contextShowList {
		sb.WriteString("\n\n")
		sb.WriteString(m.theme.Title.Render("All Project Contexts"))
		sb.WriteString("\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)))
		sb.WriteString("\n\n")

		contexts, err := workingctx.ListAll()
		if err != nil {
			sb.WriteString(m.theme.Dim.Render("Failed to load contexts: " + err.Error()))
		} else if len(contexts) == 0 {
			sb.WriteString(m.theme.Dim.Render("No contexts found."))
		} else {
			for _, ctx := range contexts {
				// Project path
				sb.WriteString(m.theme.Selected.Render("📁 " + ctx.ProjectRoot))
				sb.WriteString("\n")

				// Show Kubernetes context
				if k8s := ctx.GetKubernetes(); k8s != nil {
					k8sInfo := k8s.Context
					if k8s.Namespace != "" {
						k8sInfo += " / " + k8s.Namespace
					}
					if k8s.Kubeconfig != "" {
						k8sInfo += " (" + k8s.Kubeconfig + ")"
					}
					sb.WriteString(m.theme.Dim.Render("  ⚙️ Kubernetes: ") + m.theme.Normal.Render(k8sInfo))
					sb.WriteString("\n")
				}

				// Show AWS profile
				if aws := ctx.GetAWS(); aws != nil {
					awsInfo := aws.Profile
					if aws.Region != "" {
						awsInfo += " (" + aws.Region + ")"
					}
					sb.WriteString(m.theme.Dim.Render("  ⛅️ AWS: ") + m.theme.Normal.Render(awsInfo))
					sb.WriteString("\n")
				}

				// Show Git info
				if git := ctx.GetGit(); git != nil {
					gitInfo := ""
					if git.Branch != "" {
						gitInfo = git.Branch
						if git.Repo != "" {
							gitInfo += " @ " + git.Repo
						}
					} else if git.Repo != "" {
						gitInfo = git.Repo
					}
					if gitInfo != "" {
						sb.WriteString(m.theme.Dim.Render("  ️🌿 Git: ") + m.theme.Normal.Render(gitInfo))
						sb.WriteString("\n")
					}
				}

				// Show environment variables
				if env := ctx.GetEnv(); env != nil && len(env) > 0 {
					var envPairs []string
					for k, v := range env {
						envPairs = append(envPairs, k+"="+v)
					}
					// Show first 3, then "..." if more
					if len(envPairs) > 3 {
						envPairs = envPairs[:3]
						envPairs = append(envPairs, "...")
					}
					sb.WriteString(m.theme.Dim.Render("  🔧 Env: ") + m.theme.Normal.Render(strings.Join(envPairs, " ")))
					sb.WriteString("\n")
				}

				// Show custom values
				if custom := ctx.GetCustom(); custom != nil && len(custom) > 0 {
					var customPairs []string
					for k, v := range custom {
						customPairs = append(customPairs, k+"="+v)
					}
					// Show first 3, then "..." if more
					if len(customPairs) > 3 {
						customPairs = customPairs[:3]
						customPairs = append(customPairs, "...")
					}
					sb.WriteString(m.theme.Dim.Render("  📝 Custom: ") + m.theme.Normal.Render(strings.Join(customPairs, " ")))
					sb.WriteString("\n")
				}

				// Updated time
				sb.WriteString(m.theme.Dim.Render("  🕒 Updated: " + ctx.GetAge()))
				sb.WriteString("\n\n")
			}
		}
	}

	// Help text
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render("Press Ctrl+G for context actions"))

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

	header := m.theme.Title.Render("claude-mon") + " " + tabBar
	header = lipgloss.PlaceHorizontal(m.width, lipgloss.Left, header)

	// Two-pane layout
	minimapStr := m.renderMinimap()
	minimapWidth := 0
	if m.showMinimap {
		minimapWidth = 2
	}

	// Get left pane content first to calculate its width
	var leftContent string
	var leftBox lipgloss.Style
	if !m.hideLeftPane && m.leftPaneMode != LeftPaneModeRalph && m.leftPaneMode != LeftPaneModeContext {
		// Both panes visible - get left content
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

		leftBox = m.theme.Border
		if m.activePane == PaneLeft {
			leftBox = m.theme.ActiveBorder
		}
	}

	// Calculate pane widths based on left pane content
	var leftWidth, rightWidth int
	if m.hideLeftPane || m.leftPaneMode == LeftPaneModeRalph || m.leftPaneMode == LeftPaneModeContext {
		// Left pane hidden or in Ralph/Context mode (full-width right pane)
		leftWidth = 0
		rightWidth = m.width - 2 - minimapWidth
	} else {
		// Calculate content width with flexbox-like auto-sizing
		contentWidth := lipgloss.Width(leftContent)

		// Apply min/max constraints
		minWidth := 25 // Minimum width for left pane
		maxWidth := m.width / 2

		if contentWidth < minWidth {
			leftWidth = minWidth
		} else if contentWidth > maxWidth {
			leftWidth = maxWidth
		} else {
			leftWidth = contentWidth
		}

		// Right pane gets remaining space
		rightWidth = m.width - leftWidth - 3 - minimapWidth
	}

	// Render right pane (diff, context, or prompt preview)
	var rightContent string
	if m.leftPaneMode == LeftPaneModeContext && !m.contextEditMode {
		// Show context in full-width right pane
		rightContent = m.renderContextList()
	} else {
		rightContent = m.diffViewport.View()
	}

	rightBox := m.theme.Border
	if m.activePane == PaneRight {
		rightBox = m.theme.ActiveBorder
	}
	rightPane := rightBox.
		Width(rightWidth).
		Height(m.height - 4).
		Render(rightContent)

	var content string
	if m.hideLeftPane {
		// Only right pane visible
		if m.showMinimap {
			content = lipgloss.JoinHorizontal(lipgloss.Top, rightPane, minimapStr)
		} else {
			content = rightPane
		}
	} else {
		// Both panes visible - render left pane with calculated width
		leftPane := leftBox.
			Width(leftWidth).
			Height(m.height - 4).
			Render(leftContent)

		if m.showMinimap {
			content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane, minimapStr)
		} else {
			content = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
		}
	}

	// Always render status bar
	status := m.renderStatus()

	// Build main view
	mainView := lipgloss.JoinVertical(lipgloss.Left, header, content, status)

	// Overlay which-key popup at bottom-center when leader is active
	if m.leaderActive {
		whichKeyView := m.renderWhichKey()
		whichKeyWidth := lipgloss.Width(whichKeyView)
		whichKeyLines := strings.Split(whichKeyView, "\n")

		// Split main view into lines
		lines := strings.Split(mainView, "\n")

		// Position which-key popup 2 lines from bottom (above status bar), centered
		startLineIdx := len(lines) - 2 - len(whichKeyLines)
		if startLineIdx < 0 {
			startLineIdx = 0
		}

		// Center horizontally
		targetPos := (m.width - whichKeyWidth) / 2
		if targetPos < 0 {
			targetPos = 0
		}

		// Replace lines with centered popup content
		for i, popupLine := range whichKeyLines {
			lineIdx := startLineIdx + i
			if lineIdx >= 0 && lineIdx < len(lines) {
				// Create centered line: padding + popup line
				padding := strings.Repeat(" ", targetPos)
				lines[lineIdx] = padding + popupLine
			}
		}
		mainView = strings.Join(lines, "\n")
	}

	// Overlay context edit popup in center when editing
	if m.contextEditMode {
		popupView := m.renderContextEditPopup()
		popupWidth := lipgloss.Width(popupView)
		popupLines := strings.Split(popupView, "\n")

		// Split main view into lines
		lines := strings.Split(mainView, "\n")

		// Center popup vertically (accounting for header and status bar)
		startLineIdx := (len(lines) - len(popupLines)) / 2
		if startLineIdx < 2 {
			startLineIdx = 2 // Leave room for header
		}

		// Center horizontally
		targetPos := (m.width - popupWidth) / 2
		if targetPos < 0 {
			targetPos = 0
		}

		// Replace lines with centered popup content
		for i, popupLine := range popupLines {
			lineIdx := startLineIdx + i
			if lineIdx >= 0 && lineIdx < len(lines) {
				// Create centered line: padding + popup line
				padding := strings.Repeat(" ", targetPos)
				lines[lineIdx] = padding + popupLine
			}
		}
		mainView = strings.Join(lines, "\n")
	}

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

// renderContextEditPopup renders the centered popup for editing context values
func (m Model) renderContextEditPopup() string {
	if !m.contextEditMode {
		return ""
	}

	var content strings.Builder

	// Render based on context type
	switch m.contextEditField {
	case "k8s":
		content.WriteString(m.theme.Title.Render("⚙️ Kubernetes Context") + "\n")
		content.WriteString(m.theme.Dim.Render(strings.Repeat("─", 50)) + "\n\n")

		// Kubeconfig field
		label := "Kubeconfig:"
		if m.k8sFocusedField == 0 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.k8sKubeconfigInput.View() + "\n\n")

		// Context field
		label = "Context:"
		if m.k8sFocusedField == 1 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.k8sContextInput.View() + "\n\n")

		// Namespace field
		label = "Namespace:"
		if m.k8sFocusedField == 2 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.k8sNamespaceInput.View() + "\n")

	case "aws":
		content.WriteString(m.theme.Title.Render("⛅️ AWS Profile") + "\n")
		content.WriteString(m.theme.Dim.Render(strings.Repeat("─", 50)) + "\n\n")

		// Profile field
		label := "Profile:"
		if m.awsFocusedField == 0 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.awsProfileInput.View() + "\n\n")

		// Region field
		label = "Region:"
		if m.awsFocusedField == 1 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.awsRegionInput.View() + "\n")

	case "git":
		content.WriteString(m.theme.Title.Render("🌿 Git Info") + "\n")
		content.WriteString(m.theme.Dim.Render(strings.Repeat("─", 50)) + "\n\n")

		// Branch field
		label := "Branch:"
		if m.gitFocusedField == 0 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.gitBranchInput.View() + "\n\n")

		// Repo field
		label = "Repository:"
		if m.gitFocusedField == 1 {
			label = m.theme.Selected.Render("> " + label)
		} else {
			label = m.theme.Dim.Render("  " + label)
		}
		content.WriteString(label + "\n")
		content.WriteString("  " + m.gitRepoInput.View() + "\n")

	case "env":
		content.WriteString(m.theme.Title.Render("📦 Environment Variable") + "\n")
		content.WriteString(m.theme.Dim.Render(strings.Repeat("─", 50)) + "\n\n")
		content.WriteString(m.theme.Dim.Render("Format: KEY=value or KEY=\"value with spaces\"") + "\n\n")
		content.WriteString(m.envInput.View() + "\n")

	case "custom":
		content.WriteString(m.theme.Title.Render("🔧 Custom Value") + "\n")
		content.WriteString(m.theme.Dim.Render(strings.Repeat("─", 50)) + "\n\n")
		content.WriteString(m.theme.Dim.Render("Format: KEY=value or KEY=\"value with spaces\"") + "\n\n")
		content.WriteString(m.customInput.View() + "\n")
	}

	// Show completion overlay if active
	if m.contextCompletionActive {
		content.WriteString("\n")
		content.WriteString(m.theme.Dim.Render("─── Completions ───") + "\n")
		content.WriteString(m.contextCompletionInput.View() + "\n\n")

		// Show matches (up to 10)
		maxDisplay := 10
		startIdx := 0
		if m.contextCompletionSelected >= maxDisplay {
			startIdx = m.contextCompletionSelected - maxDisplay + 1
		}

		for i := startIdx; i < len(m.contextCompletionMatches) && i < startIdx+maxDisplay; i++ {
			candidateIdx := m.contextCompletionMatches[i]
			candidate := m.contextCompletionCandidates[candidateIdx]

			// Truncate long candidates
			if len(candidate) > 45 {
				candidate = candidate[:42] + "..."
			}

			if i == m.contextCompletionSelected {
				content.WriteString(m.theme.Selected.Render("> "+candidate) + "\n")
			} else {
				content.WriteString(m.theme.Dim.Render("  "+candidate) + "\n")
			}
		}

		if len(m.contextCompletionMatches) == 0 {
			content.WriteString(m.theme.Dim.Render("  (no matches)") + "\n")
		} else if len(m.contextCompletionMatches) > maxDisplay {
			content.WriteString(m.theme.Dim.Render(fmt.Sprintf("  ... +%d more", len(m.contextCompletionMatches)-maxDisplay)) + "\n")
		}

		content.WriteString("\n")
		content.WriteString(m.theme.Dim.Render("↑/↓:navigate  Enter:select  Esc:close"))
	} else {
		content.WriteString("\n")
		content.WriteString(m.theme.Dim.Render("Tab:next  Ctrl+@:complete  Enter:save  Esc:cancel"))
	}

	// Wrap content in a bordered box
	contentStr := content.String()

	// Style the popup with border and background
	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a4a6a")).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(1, 2).
		Width(lipgloss.Width(contentStr) + 4)

	return popupStyle.Render(contentStr)
}

// loadContextCompletions loads completion candidates for the current focused field
func (m *Model) loadContextCompletions() {
	switch m.contextEditField {
	case "k8s":
		// Load completions based on which field is focused
		switch m.k8sFocusedField {
		case 0: // kubeconfig
			m.contextCompletionCandidates = loadK8sKubeconfigs()
		case 1: // context
			// Use kubeconfig from input to find contexts
			kubeconfig := m.k8sKubeconfigInput.Value()
			if kubeconfig == "" {
				home, _ := os.UserHomeDir()
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
			m.contextCompletionCandidates = loadK8sContexts(kubeconfig)
		case 2: // namespace
			// Use kubeconfig and context from inputs
			kubeconfig := m.k8sKubeconfigInput.Value()
			if kubeconfig == "" {
				home, _ := os.UserHomeDir()
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
			context := m.k8sContextInput.Value()
			m.contextCompletionCandidates = loadK8sNamespaces(kubeconfig, context)
		}
	case "aws":
		// Load completions based on which field is focused
		switch m.awsFocusedField {
		case 0: // profile
			m.contextCompletionCandidates = loadAWSProfiles()
		case 1: // region
			m.contextCompletionCandidates = loadAWSRegions()
		}
	case "git":
		// Load completions based on which field is focused
		switch m.gitFocusedField {
		case 0: // branch
			m.contextCompletionCandidates = loadGitBranches()
		case 1: // repo
			m.contextCompletionCandidates = loadGitRepos()
		}
	case "env":
		m.contextCompletionCandidates = loadEnvCompletions()
	case "custom":
		m.contextCompletionCandidates = loadCustomCompletions(m.contextCurrent)
	default:
		m.contextCompletionCandidates = nil
	}

	// Initialize matches to all candidates
	m.contextCompletionMatches = make([]int, len(m.contextCompletionCandidates))
	for i := range m.contextCompletionCandidates {
		m.contextCompletionMatches[i] = i
	}
	m.contextCompletionSelected = 0
}

// computeContextCompletionMatches filters candidates by query
func (m *Model) computeContextCompletionMatches(query string) {
	if query == "" {
		m.contextCompletionMatches = make([]int, len(m.contextCompletionCandidates))
		for i := range m.contextCompletionCandidates {
			m.contextCompletionMatches[i] = i
		}
		return
	}

	query = strings.ToLower(query)
	m.contextCompletionMatches = nil
	for i, c := range m.contextCompletionCandidates {
		if strings.Contains(strings.ToLower(c), query) {
			m.contextCompletionMatches = append(m.contextCompletionMatches, i)
		}
	}
}

// loadK8sKubeconfigs returns available kubeconfig files from ~/.kube
func loadK8sKubeconfigs() []string {
	var results []string
	home, err := os.UserHomeDir()
	if err != nil {
		return results
	}

	kubeDir := filepath.Join(home, ".kube")

	// Find all kubeconfig files
	entries, err := os.ReadDir(kubeDir)
	if err != nil {
		return results
	}

	// Add default config first if it exists
	defaultConfig := filepath.Join(kubeDir, "config")
	if _, err := os.Stat(defaultConfig); err == nil {
		results = append(results, defaultConfig)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip non-config files (like cache, http-cache directories' contents)
		name := entry.Name()
		if name == "config" {
			continue // Already added
		}

		// Check if file looks like a kubeconfig (has contexts section)
		path := filepath.Join(kubeDir, name)
		if hasKubeconfigContexts(path) {
			results = append(results, path)
		}
	}

	return results
}

// hasKubeconfigContexts checks if a file contains a contexts: section
func hasKubeconfigContexts(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "contexts:")
}

// loadK8sContexts returns contexts from a specific kubeconfig file
func loadK8sContexts(kubeconfigPath string) []string {
	if kubeconfigPath == "" {
		return nil
	}

	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil
	}

	// Simple YAML parsing for contexts
	var results []string
	lines := strings.Split(string(data), "\n")
	inContexts := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "contexts:" {
			inContexts = true
			continue
		}

		// End of contexts section
		if inContexts && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			break
		}

		if inContexts && strings.HasPrefix(trimmed, "- name:") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
			if name != "" {
				results = append(results, name)
			}
		}
	}

	return results
}

// loadK8sNamespaces returns namespaces from the cluster using kubectl
func loadK8sNamespaces(kubeconfigPath, contextName string) []string {
	var results []string

	// Build kubectl command with kubeconfig and context
	args := []string{"get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}"}
	if kubeconfigPath != "" {
		args = append([]string{"--kubeconfig", kubeconfigPath}, args...)
	}
	if contextName != "" {
		args = append([]string{"--context", contextName}, args...)
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		// Fallback to common namespaces if kubectl fails
		return []string{"default", "kube-system", "kube-public"}
	}

	// Parse space-separated namespace names
	namespaces := strings.Fields(string(output))
	for _, ns := range namespaces {
		if ns != "" {
			results = append(results, ns)
		}
	}

	return results
}

// loadAWSCompletions returns AWS profiles from config and credentials
func loadAWSCompletions() []string {
	var results []string
	home, err := os.UserHomeDir()
	if err != nil {
		return results
	}

	// Parse ~/.aws/config for [profile xxx] sections
	configPath := filepath.Join(home, ".aws", "config")
	if profiles := parseAWSConfigProfiles(configPath); len(profiles) > 0 {
		results = append(results, profiles...)
	}

	// Parse ~/.aws/credentials for [xxx] sections (profile names without "profile " prefix)
	credsPath := filepath.Join(home, ".aws", "credentials")
	if profiles := parseAWSCredentialsProfiles(credsPath); len(profiles) > 0 {
		results = append(results, profiles...)
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, p := range results {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	return unique
}

// parseAWSConfigProfiles extracts profile names from ~/.aws/config
func parseAWSConfigProfiles(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var results []string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[profile ") && strings.HasSuffix(line, "]") {
			name := strings.TrimSuffix(strings.TrimPrefix(line, "[profile "), "]")
			results = append(results, name)
		} else if strings.HasPrefix(line, "[default]") {
			results = append(results, "default")
		}
	}

	return results
}

// parseAWSCredentialsProfiles extracts profile names from ~/.aws/credentials
func parseAWSCredentialsProfiles(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var results []string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.Trim(line, "[]")
			results = append(results, name)
		}
	}

	return results
}

// loadGitCompletions returns git branches and recent repos
func loadGitCompletions() []string {
	var results []string

	// Get current repo branches
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err == nil {
		for _, branch := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if branch != "" {
				results = append(results, branch)
			}
		}
	}

	// Add remote branches
	cmd = exec.Command("git", "branch", "-r", "--format=%(refname:short)")
	output, err = cmd.Output()
	if err == nil {
		for _, branch := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if branch != "" && !strings.Contains(branch, "HEAD") {
				// Remove origin/ prefix for cleaner display
				branch = strings.TrimPrefix(branch, "origin/")
				results = append(results, branch)
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, b := range results {
		if !seen[b] {
			seen[b] = true
			unique = append(unique, b)
		}
	}

	return unique
}

// loadEnvCompletions returns env var suggestions from zsh history
func loadEnvCompletions() []string {
	var results []string
	home, err := os.UserHomeDir()
	if err != nil {
		return results
	}

	// Try to read zsh history
	histPath := filepath.Join(home, ".zsh_history")
	data, err := os.ReadFile(histPath)
	if err != nil {
		// Try alternative location
		histPath = filepath.Join(home, ".histfile")
		data, err = os.ReadFile(histPath)
		if err != nil {
			return results
		}
	}

	// Parse history for export commands
	seen := make(map[string]bool)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		// Handle zsh extended history format (: timestamp:0;command)
		if idx := strings.Index(line, ";"); idx != -1 {
			line = line[idx+1:]
		}

		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export ") {
			// Extract KEY=VALUE
			envPart := strings.TrimPrefix(line, "export ")
			// Handle multiple exports on same line
			parts := strings.Fields(envPart)
			for _, part := range parts {
				if strings.Contains(part, "=") && !seen[part] {
					seen[part] = true
					results = append(results, part)
				}
			}
		}
	}

	// Limit results
	if len(results) > 100 {
		results = results[len(results)-100:]
	}

	return results
}

// loadCustomCompletions returns existing custom keys from current context
func loadCustomCompletions(ctx *workingctx.Context) []string {
	var results []string
	if ctx == nil {
		return results
	}

	custom := ctx.GetCustom()
	if custom == nil {
		return results
	}

	for k, v := range custom {
		results = append(results, fmt.Sprintf("%s=%s", k, v))
	}

	return results
}

// loadAWSProfiles returns AWS profiles (alias for loadAWSCompletions)
func loadAWSProfiles() []string {
	return loadAWSCompletions()
}

// loadAWSRegions returns common AWS regions
func loadAWSRegions() []string {
	return []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-west-2",
		"eu-central-1",
		"ap-northeast-1",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-south-1",
		"sa-east-1",
		"ca-central-1",
	}
}

// loadGitBranches returns git branches (alias for loadGitCompletions)
func loadGitBranches() []string {
	return loadGitCompletions()
}

// loadGitRepos returns git repository suggestions from recent history
func loadGitRepos() []string {
	var results []string

	// Get current repo remote
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err == nil {
		repo := strings.TrimSpace(string(output))
		if repo != "" {
			results = append(results, repo)
		}
	}

	// Try to get other remotes
	cmd = exec.Command("git", "remote", "-v")
	output, err = cmd.Output()
	if err == nil {
		seen := make(map[string]bool)
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				repo := fields[1]
				if !seen[repo] {
					seen[repo] = true
					results = append(results, repo)
				}
			}
		}
	}

	return results
}

// nextContextField moves focus to the next input field
func (m *Model) nextContextField() {
	switch m.contextEditField {
	case "k8s":
		m.k8sKubeconfigInput.Blur()
		m.k8sContextInput.Blur()
		m.k8sNamespaceInput.Blur()
		m.k8sFocusedField = (m.k8sFocusedField + 1) % 3
		switch m.k8sFocusedField {
		case 0:
			m.k8sKubeconfigInput.Focus()
		case 1:
			m.k8sContextInput.Focus()
		case 2:
			m.k8sNamespaceInput.Focus()
		}
	case "aws":
		m.awsProfileInput.Blur()
		m.awsRegionInput.Blur()
		m.awsFocusedField = (m.awsFocusedField + 1) % 2
		switch m.awsFocusedField {
		case 0:
			m.awsProfileInput.Focus()
		case 1:
			m.awsRegionInput.Focus()
		}
	case "git":
		m.gitBranchInput.Blur()
		m.gitRepoInput.Blur()
		m.gitFocusedField = (m.gitFocusedField + 1) % 2
		switch m.gitFocusedField {
		case 0:
			m.gitBranchInput.Focus()
		case 1:
			m.gitRepoInput.Focus()
		}
	}
}

// prevContextField moves focus to the previous input field
func (m *Model) prevContextField() {
	switch m.contextEditField {
	case "k8s":
		m.k8sKubeconfigInput.Blur()
		m.k8sContextInput.Blur()
		m.k8sNamespaceInput.Blur()
		m.k8sFocusedField = (m.k8sFocusedField + 2) % 3 // +2 to go backwards
		switch m.k8sFocusedField {
		case 0:
			m.k8sKubeconfigInput.Focus()
		case 1:
			m.k8sContextInput.Focus()
		case 2:
			m.k8sNamespaceInput.Focus()
		}
	case "aws":
		m.awsProfileInput.Blur()
		m.awsRegionInput.Blur()
		m.awsFocusedField = (m.awsFocusedField + 1) % 2 // +1 is same as -1 for mod 2
		switch m.awsFocusedField {
		case 0:
			m.awsProfileInput.Focus()
		case 1:
			m.awsRegionInput.Focus()
		}
	case "git":
		m.gitBranchInput.Blur()
		m.gitRepoInput.Blur()
		m.gitFocusedField = (m.gitFocusedField + 1) % 2
		switch m.gitFocusedField {
		case 0:
			m.gitBranchInput.Focus()
		case 1:
			m.gitRepoInput.Focus()
		}
	}
}

// setCurrentContextFieldValue sets the value of the currently focused field
func (m *Model) setCurrentContextFieldValue(value string) {
	switch m.contextEditField {
	case "k8s":
		switch m.k8sFocusedField {
		case 0:
			m.k8sKubeconfigInput.SetValue(value)
		case 1:
			m.k8sContextInput.SetValue(value)
		case 2:
			m.k8sNamespaceInput.SetValue(value)
		}
	case "aws":
		switch m.awsFocusedField {
		case 0:
			m.awsProfileInput.SetValue(value)
		case 1:
			m.awsRegionInput.SetValue(value)
		}
	case "git":
		switch m.gitFocusedField {
		case 0:
			m.gitBranchInput.SetValue(value)
		case 1:
			m.gitRepoInput.SetValue(value)
		}
	case "env":
		m.envInput.SetValue(value)
	case "custom":
		m.customInput.SetValue(value)
	}
}

// updateCurrentContextInput forwards a message to the currently focused input
func (m Model) updateCurrentContextInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.contextEditField {
	case "k8s":
		switch m.k8sFocusedField {
		case 0:
			m.k8sKubeconfigInput, cmd = m.k8sKubeconfigInput.Update(msg)
		case 1:
			m.k8sContextInput, cmd = m.k8sContextInput.Update(msg)
		case 2:
			m.k8sNamespaceInput, cmd = m.k8sNamespaceInput.Update(msg)
		}
	case "aws":
		switch m.awsFocusedField {
		case 0:
			m.awsProfileInput, cmd = m.awsProfileInput.Update(msg)
		case 1:
			m.awsRegionInput, cmd = m.awsRegionInput.Update(msg)
		}
	case "git":
		switch m.gitFocusedField {
		case 0:
			m.gitBranchInput, cmd = m.gitBranchInput.Update(msg)
		case 1:
			m.gitRepoInput, cmd = m.gitRepoInput.Update(msg)
		}
	case "env":
		m.envInput, cmd = m.envInput.Update(msg)
	case "custom":
		m.customInput, cmd = m.customInput.Update(msg)
	}
	return m, cmd
}

// saveContextEdit saves the context from the multi-field inputs
func (m *Model) saveContextEdit() {
	if m.contextCurrent == nil {
		return
	}

	switch m.contextEditField {
	case "k8s":
		kubeconfig := m.k8sKubeconfigInput.Value()
		context := m.k8sContextInput.Value()
		namespace := m.k8sNamespaceInput.Value()
		m.contextCurrent.SetKubernetes(context, namespace, kubeconfig)

	case "aws":
		profile := m.awsProfileInput.Value()
		region := m.awsRegionInput.Value()
		m.contextCurrent.SetAWS(profile, region)

	case "git":
		branch := m.gitBranchInput.Value()
		repo := m.gitRepoInput.Value()
		m.contextCurrent.SetGit(branch, repo)

	case "env":
		value := m.envInput.Value()
		if k, v, ok := parseKeyValue(value); ok {
			envVars := m.contextCurrent.GetEnv()
			if envVars == nil {
				envVars = make(map[string]string)
			}
			envVars[k] = v
			m.contextCurrent.SetEnv(envVars)
		}

	case "custom":
		value := m.customInput.Value()
		if k, v, ok := parseKeyValue(value); ok {
			customVars := m.contextCurrent.GetCustom()
			if customVars == nil {
				customVars = make(map[string]string)
			}
			customVars[k] = v
			m.contextCurrent.SetCustom(customVars)
		}
	}

	// Save the context
	if err := m.contextCurrent.Save(); err != nil {
		m.addToast(fmt.Sprintf("Failed to save context: %v", err), ToastError)
		return
	}

	m.addToast("Context updated", ToastSuccess)
}

// listVisibleItems returns the number of items that can fit in the history list view
func (m Model) listVisibleItems() int {
	// Account for header (2 lines: title + separator) and footer indicator
	listHeight := m.height - 6 // status bar, tabs, header, margins
	availableHeight := listHeight - 3
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// ensureSelectedVisible adjusts listScrollOffset to keep selected item visible
func (m *Model) ensureSelectedVisible() {
	if len(m.changes) == 0 {
		return
	}

	totalItems := len(m.changes)
	visibleItems := m.listVisibleItems()

	// Convert selectedIndex to visual position (list is displayed in reverse)
	// Visual position 0 = data index len-1, visual position N = data index len-1-N
	// So: visualPos = totalItems - 1 - selectedIndex
	visualPos := totalItems - 1 - m.selectedIndex

	// If selected is above visible area (scrolled past), scroll up
	if visualPos < m.listScrollOffset {
		m.listScrollOffset = visualPos
	}

	// If selected is below visible area, scroll down
	if visualPos >= m.listScrollOffset+visibleItems {
		m.listScrollOffset = visualPos - visibleItems + 1
	}

	// Clamp scroll offset
	maxOffset := totalItems - visibleItems
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.listScrollOffset > maxOffset {
		m.listScrollOffset = maxOffset
	}
	if m.listScrollOffset < 0 {
		m.listScrollOffset = 0
	}
}

func (m Model) renderHistory() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("No changes yet...\nWaiting for Claude edits")
	}

	var sb strings.Builder

	// Calculate visible items (account for header: title + separator = 2 lines)
	visibleItems := m.listVisibleItems()
	totalItems := len(m.changes)

	// Show scroll indicator in header if scrollable
	if totalItems > visibleItems {
		scrollInfo := fmt.Sprintf(" [%d-%d/%d]", m.listScrollOffset+1,
			min(m.listScrollOffset+visibleItems, totalItems), totalItems)
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("History (%d)%s\n", totalItems, scrollInfo)))
	} else {
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("History (%d)\n", totalItems)))
	}
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 20)) + "\n")

	// Calculate available width for path in history pane
	historyWidth := m.width / 3
	pathWidth := historyWidth - 15 // Account for timestamp, tool, prefix

	// Track current commit for grouping
	currentCommit := ""

	// Display in reverse order: newest (highest index) first
	// listScrollOffset is visual offset from top (0 = showing newest)
	// Visual position 0 = data index len-1, visual position N = data index len-1-N
	startVisual := m.listScrollOffset
	endVisual := startVisual + visibleItems
	if endVisual > totalItems {
		endVisual = totalItems
	}

	// Iterate through visible items (newest first in display)
	for visualPos := startVisual; visualPos < endVisual; visualPos++ {
		// Convert visual position to data index (reverse mapping)
		i := totalItems - 1 - visualPos
		if i < 0 {
			break
		}

		change := m.changes[i]

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

	// Show scroll indicator at bottom if there's more content
	if m.listScrollOffset+visibleItems < totalItems {
		sb.WriteString(m.theme.Dim.Render("  ↓ more...") + "\n")
	}

	return sb.String()
}

// renderPromptsList renders the prompts list for the left pane
func (m Model) renderPromptsList() string {
	var sb strings.Builder
	listWidth := m.width / 3

	// Show fuzzy filter overlay when active
	if m.promptFuzzyActive {
		sb.WriteString(m.theme.Title.Render("Filter Prompts") + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

		// Search input
		sb.WriteString(m.promptFuzzyInput.View() + "\n\n")

		// Show matching prompts
		if len(m.promptFuzzyMatches) == 0 {
			sb.WriteString(m.theme.Dim.Render("No matches"))
		} else {
			// Show up to 10 matches
			maxShow := 10
			if len(m.promptFuzzyMatches) < maxShow {
				maxShow = len(m.promptFuzzyMatches)
			}
			for i := 0; i < maxShow; i++ {
				idx := m.promptFuzzyMatches[i]
				p := m.promptFilteredList[idx]
				prefix := "  "
				if i == m.promptFuzzySelected {
					prefix = "> "
				}
				scope := "[P]"
				if p.IsGlobal {
					scope = "[G]"
				}
				line := fmt.Sprintf("%s%s %s", prefix, scope, p.Name)
				if len(line) > listWidth-4 {
					line = line[:listWidth-7] + "..."
				}
				if i == m.promptFuzzySelected {
					sb.WriteString(m.theme.Selected.Render(line) + "\n")
				} else {
					sb.WriteString(m.theme.Normal.Render(line) + "\n")
				}
			}
			if len(m.promptFuzzyMatches) > maxShow {
				sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("  ...and %d more", len(m.promptFuzzyMatches)-maxShow)) + "\n")
			}
		}

		sb.WriteString("\n" + m.theme.Dim.Render("Enter:select  Esc:cancel  ↑/↓:navigate"))
		return sb.String()
	}

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
		// Build header with filter scope indicator
		filterIndicator := ""
		switch m.promptFilter {
		case PromptFilterProject:
			filterIndicator = " [Project]"
		case PromptFilterGlobal:
			filterIndicator = " [Global]"
		}
		header := fmt.Sprintf("Prompts (%d)%s", len(m.promptFilteredList), filterIndicator)
		sb.WriteString(m.theme.Title.Render(header) + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n")

		if len(m.promptFilteredList) == 0 {
			if m.promptFilter != PromptFilterAll {
				sb.WriteString(m.theme.Dim.Render("No matching prompts\nPress '/' to change filter"))
			} else {
				sb.WriteString(m.theme.Dim.Render("No prompts\nPress 'n' to create"))
			}
		} else {
			for i, p := range m.promptFilteredList {
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

	// If FileContent is empty (e.g., loaded from history), try to retrieve it
	if change.FileContent == "" && change.FilePath != "" && change.ToolName != "Write" {
		var fileContent string
		var err error
		var source string

		// Make file path absolute if it's relative
		filePath := change.FilePath
		if !filepath.IsAbs(filePath) {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				filePath = filepath.Join(cwd, filePath)
			}
		}

		// Try VCS-based retrieval if we have commit info
		if change.CommitSHA != "" && change.VCSType != "" {
			// Get workspace root from current directory (more reliable than file path)
			cwd, cwdErr := os.Getwd()
			if cwdErr == nil {
				if workspaceRoot, rootErr := vcs.GetWorkspaceRoot(cwd, change.VCSType); rootErr == nil {
					fileContent, err = vcs.GetFileAtCommit(workspaceRoot, filePath, change.CommitSHA, change.VCSType)
					if err == nil {
						source = fmt.Sprintf("VCS (%s@%s)", change.VCSType, change.CommitSHA[:min(8, len(change.CommitSHA))])
					}
				}
			}
		}

		// Fall back to reading current file if VCS retrieval failed
		if fileContent == "" {
			if content, readErr := os.ReadFile(filePath); readErr == nil {
				fileContent = string(content)
				source = "current file"
			} else {
				err = readErr
			}
		}

		if fileContent != "" {
			change.FileContent = fileContent
			// Update the stored change so we don't re-read every time
			m.changes[m.selectedIndex] = change
			logger.Log("Retrieved file content for history entry: %s (%d bytes, source: %s)", change.FilePath, len(change.FileContent), source)
		} else {
			logger.Log("Failed to retrieve file for history entry: %s: %v", change.FilePath, err)
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
	// In Ralph mode, use the full-width renderer
	return m.renderRalphFull()
}

// renderRalphFull renders a combined full-width Ralph view (status + prompt)
func (m *Model) renderRalphFull() string {
	var sb strings.Builder

	if m.ralphState == nil || !m.ralphState.Active {
		sb.WriteString(m.theme.Title.Render("Ralph Loop") + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", m.width-4)) + "\n\n")
		sb.WriteString(m.theme.Dim.Render("No active Ralph loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start a Ralph loop with:\n"))
		sb.WriteString(m.theme.Normal.Render("  /ralph-loop\n\n"))
		return sb.String()
	}

	// Status section at top
	sb.WriteString(m.theme.Title.Render("Ralph Loop Status") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", m.width-4)) + "\n\n")

	// Active status
	if m.ralphState.Active {
		sb.WriteString(m.theme.Selected.Render("🔄 Active") + "  ")

		// Iteration progress
		progress := fmt.Sprintf("Iteration: %d/%d", m.ralphState.Iteration, m.ralphState.MaxIterations)
		sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

		// Completion promise
		if m.ralphState.Promise != "" {
			sb.WriteString(m.theme.Dim.Render("Promise: ") + m.theme.Normal.Render("\""+m.ralphState.Promise+"\"") + "\n\n")
		}

		// Started at
		if !m.ralphState.StartedAt.IsZero() {
			durationStr := ralph.FormatDuration(time.Since(m.ralphState.StartedAt))
			sb.WriteString(m.theme.Dim.Render("Started: ") + m.theme.Normal.Render(durationStr) + "\n\n")
		}

		// State file location
		if m.ralphState.Path != "" {
			sb.WriteString(m.theme.Dim.Render("State: ") + m.theme.Normal.Render(m.ralphState.Path) + "\n\n")
		}
	}

	// Prompt content section
	sb.WriteString(m.theme.Title.Render("Loop Prompt") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", m.width-4)) + "\n\n")

	if m.ralphState.Prompt == "" {
		sb.WriteString(m.theme.Dim.Render("No prompt content"))
		return sb.String()
	}

	// Render prompt as markdown
	rendered, err := m.renderMarkdown(m.ralphState.Prompt, m.width-4)
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

	// Calculate viewport width based on left pane visibility
	var vpWidth int
	if m.hideLeftPane {
		vpWidth = m.width - 4 - minimapWidth
	} else {
		leftWidth := m.width / 3
		vpWidth = m.width - leftWidth - 6 - minimapWidth
	}

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
	k := m.config.Keys

	// Plan input mode
	if m.planInputActive {
		return m.theme.Status.Render("Enter:submit  Esc:cancel")
	}
	if m.planGenerating {
		return m.theme.Status.Render("Generating plan...")
	}

	// Simplified status bar - just nav + leader key hint
	var modeName string
	switch m.leftPaneMode {
	case LeftPaneModeHistory:
		modeName = "History"
	case LeftPaneModePrompts:
		modeName = "Prompts"
	case LeftPaneModeRalph:
		modeName = "Ralph"
	case LeftPaneModePlan:
		modeName = "Plan"
	case LeftPaneModeContext:
		modeName = "Context"
	}

	paneIndicator := "L"
	if m.activePane == PaneRight {
		paneIndicator = "R"
	}

	// Socket connection indicator (local nvim socket)
	socketIndicator := "○" // Disconnected/no recent activity
	socketStyle := m.theme.Dim
	if m.socketConnected {
		if time.Since(m.lastMsgTime) < 30*time.Second {
			socketIndicator = "●" // Connected with recent activity
			socketStyle = m.theme.Added
		} else {
			socketIndicator = "◐" // Connected but idle
			socketStyle = m.theme.Modified
		}
	}

	// Daemon connection indicator
	daemonIndicator := "○" // Not connected
	daemonStyle := m.theme.Dim
	if m.daemonConnected {
		if m.daemonWorkspaceActive && time.Since(m.daemonLastActivity) < 5*time.Minute {
			daemonIndicator = "●" // Connected with recent workspace activity
			daemonStyle = m.theme.Added
		} else if m.daemonWorkspaceActive {
			daemonIndicator = "◐" // Connected, workspace tracked but idle
			daemonStyle = m.theme.Modified
		} else {
			daemonIndicator = "◑" // Connected but workspace not tracked
			daemonStyle = m.theme.Dim
		}
	}

	// Build status: left side info, right side indicators
	leftStatus := fmt.Sprintf(
		"%s [%s]  %s/%s:nav  Tab:mode  [/]:pane  ^G:menu",
		modeName, paneIndicator, k.Down, k.Up)

	// Build right side: daemon indicator + socket indicator
	rightPart := daemonStyle.Render("D"+daemonIndicator) + " " + socketStyle.Render("S"+socketIndicator)
	rightLen := 5 // "D● S●" = 5 chars

	// Calculate padding to push indicators to right
	statusWidth := m.width - 2
	leftLen := len(leftStatus)

	padding := statusWidth - leftLen - rightLen
	if padding < 1 {
		padding = 1
	}

	return m.theme.Status.Render(leftStatus + strings.Repeat(" ", padding) + rightPart)
}

func (m Model) renderHelp() string {
	k := m.config.Keys
	var help strings.Builder

	help.WriteString("\n  claude-mon TUI - Help\n\n")

	// Global section (always shown)
	help.WriteString("  === Global ===\n")
	help.WriteString(fmt.Sprintf("    %-14s Cycle tabs\n", k.NextTab+"/"+k.PrevTab))
	help.WriteString("    1-4            Direct tab access\n")
	if !m.hideLeftPane {
		help.WriteString(fmt.Sprintf("    %-14s Switch pane focus\n", k.LeftPane+" / "+k.RightPane))
	}
	help.WriteString(fmt.Sprintf("    %-14s Toggle left pane\n", k.ToggleLeftPane))
	help.WriteString(fmt.Sprintf("    %-14s Toggle minimap\n", k.ToggleMinimap))
	help.WriteString(fmt.Sprintf("    %-14s This help\n", k.Help))
	help.WriteString(fmt.Sprintf("    %-14s Quit\n\n", k.Quit))

	// Mode-specific section
	switch m.leftPaneMode {
	case LeftPaneModeHistory:
		help.WriteString("  === History Mode ===\n")
		help.WriteString(fmt.Sprintf("    %-14s Next/previous change\n", k.Next+"/"+k.Prev))
		help.WriteString(fmt.Sprintf("    %-14s Scroll diff\n", k.Down+"/"+k.Up))
		help.WriteString(fmt.Sprintf("    %-14s Scroll horizontally\n", k.ScrollLeft+"/"+k.ScrollRight))
		help.WriteString(fmt.Sprintf("    %-14s Open file in nvim at line\n", k.OpenInNvim))
		help.WriteString(fmt.Sprintf("    %-14s Open file in nvim\n", k.OpenNvimCwd))
		help.WriteString(fmt.Sprintf("    %-14s Clear history\n\n", k.ClearHistory))

	case LeftPaneModePrompts:
		if m.promptShowVersions {
			help.WriteString("  === Version View ===\n")
			help.WriteString(fmt.Sprintf("    %-14s Navigate versions\n", k.Down+"/"+k.Up))
			help.WriteString(fmt.Sprintf("    %-14s Revert to version\n", k.RevertVersion+"/"+k.SendPrompt))
			help.WriteString(fmt.Sprintf("    %-14s View version (read-only)\n", k.EditPrompt))
			help.WriteString(fmt.Sprintf("    %-14s Delete version\n", k.DeletePrompt))
			help.WriteString(fmt.Sprintf("    %-14s Back to prompts\n\n", k.ViewVersions+"/Esc"))
		} else {
			help.WriteString("  === Prompts Mode ===\n")
			help.WriteString(fmt.Sprintf("    %-14s New project prompt\n", k.NewPrompt))
			help.WriteString(fmt.Sprintf("    %-14s New global prompt\n", k.NewGlobalPrompt))
			help.WriteString(fmt.Sprintf("    %-14s Edit selected prompt\n", k.EditPrompt))
			help.WriteString(fmt.Sprintf("    %-14s Create version backup\n", k.CreateVersion))
			help.WriteString(fmt.Sprintf("    %-14s View all versions\n", k.ViewVersions))
			help.WriteString(fmt.Sprintf("    %-14s Delete prompt\n", k.DeletePrompt))
			help.WriteString(fmt.Sprintf("    %-14s Yank (copy to clipboard)\n", k.YankPrompt))
			help.WriteString(fmt.Sprintf("    %-14s Cycle inject method\n", k.InjectMethod))
			help.WriteString(fmt.Sprintf("    %-14s Inject prompt\n\n", k.SendPrompt))
		}

	case LeftPaneModeRalph:
		help.WriteString("  === Ralph Mode ===\n")
		if m.ralphState != nil && m.ralphState.Active {
			help.WriteString(fmt.Sprintf("    %-14s Cancel Ralph loop\n", k.CancelRalph))
		}
		help.WriteString(fmt.Sprintf("    %-14s Refresh status\n", k.Refresh))
		help.WriteString(fmt.Sprintf("    %-14s Scroll prompt\n\n", k.Down+"/"+k.Up))

	case LeftPaneModePlan:
		help.WriteString("  === Plan Mode ===\n")
		help.WriteString(fmt.Sprintf("    %-14s Generate new plan\n", k.GeneratePlan))
		if m.planPath != "" {
			help.WriteString(fmt.Sprintf("    %-14s Edit plan in nvim\n", k.EditPlan))
		}
		help.WriteString(fmt.Sprintf("    %-14s Refresh plan\n", k.Refresh))
		help.WriteString(fmt.Sprintf("    %-14s Scroll plan content\n\n", k.Down+"/"+k.Up+"/"+k.PageDown+"/"+k.PageUp))
	}

	// Template variables (only in prompts mode)
	if m.leftPaneMode == LeftPaneModePrompts && !m.promptShowVersions {
		help.WriteString("  === Template Variables ===\n")
		help.WriteString("    {{plan}}       Plan file content\n")
		help.WriteString("    {{plan_name}}  Plan file name\n")
		help.WriteString("    {{file}}       Current file path\n")
		help.WriteString("    {{file_name}}  Current file name\n")
		help.WriteString("    {{project}}    Project directory name\n")
		help.WriteString("    {{cwd}}        Working directory\n\n")
	}

	help.WriteString("  Press any key to close help\n")

	return m.theme.Help.Render(help.String())
}

// renderHelpBar renders a compact help bar using bubbles/help
func (m Model) renderHelpBar() string {
	// Get mode name for mode-specific keybindings
	mode := ""
	switch m.leftPaneMode {
	case LeftPaneModeHistory:
		mode = "history"
	case LeftPaneModePrompts:
		mode = "prompts"
	case LeftPaneModeRalph:
		mode = "ralph"
	case LeftPaneModePlan:
		mode = "plan"
	case LeftPaneModeContext:
		mode = "context"
	}

	// Use ModeKeyMap for mode-specific help
	modeKeyMap := NewModeKeyMap(m.keyMap, mode)
	return m.help.View(modeKeyMap)
}

// WhichKeyItem represents a single item in the which-key popup
type WhichKeyItem struct {
	Key         string
	Description string
	IsGroup     bool // true if this leads to another menu level
}

// renderWhichKey renders context-sensitive which-key popup as a floating box
func (m Model) renderWhichKey() string {
	var contextItems []WhichKeyItem

	// Context-sensitive actions based on pane and mode (fuller descriptions)
	var context string
	if m.activePane == PaneRight {
		context = "FILE VIEWER"
		contextItems = []WhichKeyItem{
			{Key: "g", Description: "open in nvim at line"},
			{Key: "o", Description: "open file in nvim"},
		}
	} else {
		switch m.leftPaneMode {
		case LeftPaneModeHistory:
			context = "HISTORY"
			contextItems = []WhichKeyItem{
				{Key: "g", Description: "open in nvim at line"},
				{Key: "o", Description: "open file in nvim"},
				{Key: "x", Description: "clear history"},
			}
		case LeftPaneModePrompts:
			context = "PROMPTS"
			contextItems = []WhichKeyItem{
				{Key: "n", Description: "new prompt"},
				{Key: "N", Description: "new global prompt"},
				{Key: "e", Description: "edit selected"},
				{Key: "y", Description: "yank to clipboard"},
				{Key: "d", Description: "delete prompt"},
				{Key: "i", Description: "injection method"},
				{Key: "⏎", Description: "inject prompt"},
				{Key: "s", Description: "run as objective"},
			}
		case LeftPaneModeRalph:
			context = "RALPH LOOP"
			contextItems = []WhichKeyItem{
				{Key: "C", Description: "cancel loop"},
				{Key: "r", Description: "refresh status"},
			}
		case LeftPaneModePlan:
			context = "PLAN"
			contextItems = []WhichKeyItem{
				{Key: "G", Description: "generate new plan"},
				{Key: "e", Description: "edit in nvim"},
				{Key: "r", Description: "refresh view"},
				{Key: "s", Description: "run plan"},
			}
		case LeftPaneModeContext:
			context = "CONTEXT"
			contextItems = []WhichKeyItem{
				{Key: "k", Description: "set Kubernetes"},
				{Key: "a", Description: "set AWS"},
				{Key: "g", Description: "set Git"},
				{Key: "e", Description: "set Env var"},
				{Key: "c", Description: "set Custom"},
				{Key: "K", Description: "clear K8s"},
				{Key: "A", Description: "clear AWS"},
				{Key: "G", Description: "clear Git"},
				{Key: "E", Description: "clear Env"},
				{Key: "X", Description: "clear Custom"},
				{Key: "C", Description: "clear all"},
				{Key: "r", Description: "reload"},
				{Key: "l", Description: "list all"},
			}
		}
	}

	// Styles
	boxStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1a2e")).
		Foreground(lipgloss.Color("#e0e0e0")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a4a6a")).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffd700")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e0e0e0"))

	dimKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#b8860b"))

	dimDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00d4aa")).
		Bold(true)

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4a4a6a"))

	// Fixed column width for alignment
	const colWidth = 24

	// Helper to pad string to column width (safe for negative values)
	padToWidth := func(s string, width int) string {
		w := lipgloss.Width(s)
		if w >= width {
			return s + "  " // minimum spacing
		}
		return s + strings.Repeat(" ", width-w)
	}

	var lines []string

	// Header
	lines = append(lines, headerStyle.Render(context))

	// Context items in 2 columns
	for i := 0; i < len(contextItems); i += 2 {
		left := fmt.Sprintf("%s  %s",
			keyStyle.Render(contextItems[i].Key),
			descStyle.Render(contextItems[i].Description))

		var line string
		if i+1 < len(contextItems) {
			right := fmt.Sprintf("%s  %s",
				keyStyle.Render(contextItems[i+1].Key),
				descStyle.Render(contextItems[i+1].Description))
			line = padToWidth(left, colWidth) + right
		} else {
			line = left
		}
		lines = append(lines, line)
	}

	// Separator
	lines = append(lines, separatorStyle.Render(strings.Repeat("─", colWidth*2)))

	// Global actions in 2 columns
	globalItems := []WhichKeyItem{
		{Key: "h", Description: "toggle pane"},
		{Key: "m", Description: "toggle minimap"},
		{Key: "1-4", Description: "switch mode"},
		{Key: "?", Description: "full help"},
		{Key: "q", Description: "quit"},
	}
	for i := 0; i < len(globalItems); i += 2 {
		left := fmt.Sprintf("%s  %s",
			dimKeyStyle.Render(globalItems[i].Key),
			dimDescStyle.Render(globalItems[i].Description))

		var line string
		if i+1 < len(globalItems) {
			right := fmt.Sprintf("%s  %s",
				dimKeyStyle.Render(globalItems[i+1].Key),
				dimDescStyle.Render(globalItems[i+1].Description))
			line = padToWidth(left, colWidth) + right
		} else {
			line = left
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return boxStyle.Render(content)
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

// parseKeyValue parses a KEY=VALUE string where VALUE can be quoted with ", ', or `
// Examples:
//   - "foo=bar" -> ("foo", "bar")
//   - "foo='hello world'" -> ("foo", "hello world")
//   - `foo="my sentence"` -> ("foo", "my sentence")
//   - "foo=`backtick quoted`" -> ("foo", "backtick quoted")
func parseKeyValue(input string) (key, value string, ok bool) {
	// Find the first = sign
	eqIdx := -1
	for i, c := range input {
		if c == '=' {
			eqIdx = i
			break
		}
	}

	if eqIdx <= 0 {
		return "", "", false
	}

	key = input[:eqIdx]
	remainder := input[eqIdx+1:]

	// Check if value is quoted
	if len(remainder) >= 2 {
		first := remainder[0]
		if first == '"' || first == '\'' || first == '`' {
			// Find matching closing quote
			for i := len(remainder) - 1; i > 0; i-- {
				if remainder[i] == first {
					value = remainder[1:i]
					return key, value, true
				}
			}
		}
	}

	// Not quoted, use the whole remainder
	value = remainder
	return key, value, true
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
	m.applyPromptFilter()
}

// applyPromptFilter filters the prompt list based on current filter scope
func (m *Model) applyPromptFilter() {
	if m.promptFilter == PromptFilterAll {
		m.promptFilteredList = m.promptList
	} else {
		m.promptFilteredList = make([]prompt.Prompt, 0)
		for _, p := range m.promptList {
			switch m.promptFilter {
			case PromptFilterProject:
				if !p.IsGlobal {
					m.promptFilteredList = append(m.promptFilteredList, p)
				}
			case PromptFilterGlobal:
				if p.IsGlobal {
					m.promptFilteredList = append(m.promptFilteredList, p)
				}
			}
		}
	}
	// Adjust selection if needed
	if m.promptSelected >= len(m.promptFilteredList) {
		if len(m.promptFilteredList) > 0 {
			m.promptSelected = len(m.promptFilteredList) - 1
		} else {
			m.promptSelected = 0
		}
	}
}

// computeFuzzyMatches returns indices of prompts matching the query
func (m *Model) computeFuzzyMatches(query string) []int {
	if query == "" {
		// Empty query matches all
		matches := make([]int, len(m.promptFilteredList))
		for i := range m.promptFilteredList {
			matches[i] = i
		}
		return matches
	}

	query = strings.ToLower(query)
	var matches []int
	for i, p := range m.promptFilteredList {
		name := strings.ToLower(p.Name)
		desc := strings.ToLower(p.Description)
		// Simple substring match (fuzzy-ish)
		if strings.Contains(name, query) || strings.Contains(desc, query) {
			matches = append(matches, i)
		}
	}
	return matches
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
