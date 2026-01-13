package highlight

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/ztaylor/claude-mon/internal/theme"
)

// Highlighter provides syntax highlighting for code
type Highlighter struct {
	theme *theme.Theme
	style *chroma.Style
	cache *Cache
}

// NewHighlighter creates a new highlighter with the given theme
func NewHighlighter(t *theme.Theme) *Highlighter {
	style := styles.Get(t.ChromaStyle)
	if style == nil {
		style = styles.Fallback
	}

	return &Highlighter{
		theme: t,
		style: style,
		cache: NewCache(100), // LRU cache with 100 entries
	}
}

// Theme returns the current theme
func (h *Highlighter) Theme() *theme.Theme {
	return h.theme
}

// Highlight applies syntax highlighting to code based on filename
func (h *Highlighter) Highlight(code, filename string) string {
	if code == "" {
		return code
	}

	// Check cache first
	cacheKey := h.cacheKey(filename, code)
	if cached, ok := h.cache.Get(cacheKey); ok {
		return cached
	}

	// Get lexer for file type
	lexer := h.getLexer(filename, code)
	if lexer == nil {
		return code // No highlighting available
	}

	// Tokenize
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	// Render tokens with styling
	result := h.renderTokens(iterator)

	// Cache the result
	h.cache.Set(cacheKey, result)

	return result
}

// HighlightLine highlights a single line of code
func (h *Highlighter) HighlightLine(line, filename string) string {
	return h.Highlight(line, filename)
}

func (h *Highlighter) getLexer(filename, code string) chroma.Lexer {
	// Try by filename first
	lexer := lexers.Match(filename)
	if lexer != nil {
		return chroma.Coalesce(lexer)
	}

	// Try by extension
	ext := filepath.Ext(filename)
	if ext != "" {
		lexer = lexers.Get(ext[1:]) // Remove leading dot
		if lexer != nil {
			return chroma.Coalesce(lexer)
		}
	}

	// Try to analyze content
	lexer = lexers.Analyse(code)
	if lexer != nil {
		return chroma.Coalesce(lexer)
	}

	return nil
}

func (h *Highlighter) renderTokens(iterator chroma.Iterator) string {
	var sb strings.Builder

	for _, token := range iterator.Tokens() {
		style := h.tokenStyle(token.Type)
		sb.WriteString(style.Render(token.Value))
	}

	return sb.String()
}

func (h *Highlighter) tokenStyle(tokenType chroma.TokenType) lipgloss.Style {
	// Map Chroma token types to theme styles
	switch {
	case tokenType == chroma.Keyword || tokenType.InCategory(chroma.Keyword):
		return h.theme.Keyword
	case tokenType == chroma.String || tokenType.InCategory(chroma.LiteralString):
		return h.theme.String
	case tokenType == chroma.Number || tokenType.InCategory(chroma.LiteralNumber):
		return h.theme.Number
	case tokenType == chroma.Comment || tokenType.InCategory(chroma.Comment):
		return h.theme.Comment
	case tokenType == chroma.NameFunction || tokenType == chroma.NameBuiltin:
		return h.theme.Function
	case tokenType == chroma.NameClass || tokenType == chroma.NameNamespace || tokenType == chroma.KeywordType:
		return h.theme.Type
	case tokenType == chroma.Operator || tokenType.InCategory(chroma.Operator):
		return h.theme.Operator
	case tokenType == chroma.Punctuation:
		return h.theme.Punctuation
	default:
		return h.theme.Normal
	}
}

func (h *Highlighter) cacheKey(filename, code string) string {
	// Use filename + first 100 chars of code as key
	prefix := code
	if len(prefix) > 100 {
		prefix = prefix[:100]
	}
	return filename + ":" + prefix
}
