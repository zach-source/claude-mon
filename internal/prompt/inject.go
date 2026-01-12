package prompt

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// InjectionMethod represents how to send the prompt
type InjectionMethod int

const (
	InjectTmux     InjectionMethod = iota // Send to tmux pane
	InjectOSC52                           // OSC 52 terminal clipboard
	InjectClipboard                       // System clipboard (pbcopy/xclip/xsel)
)

// Inject sends the prompt content using the specified method
func Inject(content string, method InjectionMethod) error {
	switch method {
	case InjectTmux:
		return injectTmux(content)
	case InjectOSC52:
		return injectOSC52(content)
	case InjectClipboard:
		return injectClipboard(content)
	default:
		return fmt.Errorf("unknown injection method: %d", method)
	}
}

// injectTmux sends content to the active tmux pane using send-keys
func injectTmux(content string) error {
	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not running inside tmux")
	}

	// Escape special characters for tmux
	// send-keys interprets certain sequences, so we use -l for literal
	cmd := exec.Command("tmux", "send-keys", "-l", content)
	return cmd.Run()
}

// injectOSC52 copies content using OSC 52 escape sequence
// This works over SSH and in terminals that support it
func injectOSC52(content string) error {
	// OSC 52 format: ESC ] 52 ; c ; <base64-content> ST
	// ST (String Terminator) is ESC \ or BEL (\a)
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	// Write to stdout/terminal
	// Use BEL (\a) as terminator for wider compatibility
	seq := fmt.Sprintf("\x1b]52;c;%s\a", encoded)

	// Write directly to the terminal
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		// Fallback to stdout
		_, err = os.Stdout.WriteString(seq)
		return err
	}
	defer tty.Close()

	_, err = tty.WriteString(seq)
	return err
}

// injectClipboard copies content to system clipboard
func injectClipboard(content string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			// Wayland
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip, xsel, or wl-copy)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

// DetectBestMethod returns the best available injection method
func DetectBestMethod() InjectionMethod {
	// If in tmux, prefer that
	if os.Getenv("TMUX") != "" {
		return InjectTmux
	}

	// Check for OSC 52 support via TERM
	term := os.Getenv("TERM")
	// Many modern terminals support OSC 52
	if strings.Contains(term, "xterm") ||
		strings.Contains(term, "screen") ||
		strings.Contains(term, "tmux") ||
		os.Getenv("TERM_PROGRAM") == "iTerm.app" ||
		os.Getenv("TERM_PROGRAM") == "WezTerm" ||
		os.Getenv("TERM_PROGRAM") == "Ghostty" ||
		os.Getenv("KITTY_WINDOW_ID") != "" {
		return InjectOSC52
	}

	// Fallback to system clipboard
	return InjectClipboard
}

// MethodName returns a human-readable name for the injection method
func MethodName(method InjectionMethod) string {
	switch method {
	case InjectTmux:
		return "tmux"
	case InjectOSC52:
		return "OSC 52"
	case InjectClipboard:
		return "clipboard"
	default:
		return "unknown"
	}
}
