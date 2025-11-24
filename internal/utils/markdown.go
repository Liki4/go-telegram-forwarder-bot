package utils

import (
	"strings"
)

// EscapeMarkdown escapes special characters in Markdown format for Telegram
// Special characters that need escaping: _ * [ ] ( ) ` ~
func EscapeMarkdown(text string) string {
	// Replace special characters with escaped versions
	text = strings.ReplaceAll(text, "\\", "\\\\") // Escape backslash first
	text = strings.ReplaceAll(text, "_", "\\_")   // Escape underscore
	text = strings.ReplaceAll(text, "*", "\\*")   // Escape asterisk
	text = strings.ReplaceAll(text, "[", "\\[")   // Escape left bracket
	text = strings.ReplaceAll(text, "]", "\\]")   // Escape right bracket
	text = strings.ReplaceAll(text, "(", "\\(")   // Escape left parenthesis
	text = strings.ReplaceAll(text, ")", "\\)")   // Escape right parenthesis
	text = strings.ReplaceAll(text, "`", "\\`")   // Escape backtick
	text = strings.ReplaceAll(text, "~", "\\~")   // Escape tilde
	return text
}
