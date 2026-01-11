package diff

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// WordDiff represents a word-level diff segment
type WordDiff struct {
	Type DiffType
	Text string
}

// DiffType represents the type of diff operation
type DiffType int

const (
	DiffEqual DiffType = iota
	DiffInsert
	DiffDelete
)

// ComputeWordDiff generates a word-level diff between two strings
func ComputeWordDiff(oldText, newText string) []WordDiff {
	dmp := diffmatchpatch.New()

	// Use line mode for better performance on larger texts
	a, b, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	// Clean up for better word-level results
	diffs = dmp.DiffCleanupSemantic(diffs)

	result := make([]WordDiff, 0, len(diffs))
	for _, d := range diffs {
		var diffType DiffType
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			diffType = DiffEqual
		case diffmatchpatch.DiffInsert:
			diffType = DiffInsert
		case diffmatchpatch.DiffDelete:
			diffType = DiffDelete
		}
		result = append(result, WordDiff{
			Type: diffType,
			Text: d.Text,
		})
	}

	return result
}

// ComputeInlineWordDiff computes word-level changes within a single line pair
func ComputeInlineWordDiff(oldLine, newLine string) []WordDiff {
	dmp := diffmatchpatch.New()

	// Split by words for finer granularity
	diffs := dmp.DiffMain(oldLine, newLine, false)
	diffs = dmp.DiffCleanupSemanticLossless(diffs)

	result := make([]WordDiff, 0, len(diffs))
	for _, d := range diffs {
		var diffType DiffType
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			diffType = DiffEqual
		case diffmatchpatch.DiffInsert:
			diffType = DiffInsert
		case diffmatchpatch.DiffDelete:
			diffType = DiffDelete
		}
		result = append(result, WordDiff{
			Type: diffType,
			Text: d.Text,
		})
	}

	return result
}

// SplitLines splits text into lines, preserving empty lines
func SplitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Remove trailing empty line if text ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
