package proc

import (
	"deep_in_memory/internal/data"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractLabel_Empty(t *testing.T) {
	fields := []string{"addr", "rw-p", "00000000", "00:00", "0"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "", label)
	assert.Equal(t, data.Empty, labelType)
}

func TestExtractLabel_TooFewFields(t *testing.T) {
	fields := []string{"addr", "rw-p"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "", label)
	assert.Equal(t, data.Empty, labelType)
}

func TestExtractLabel_BracketedLabel(t *testing.T) {
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "[heap]"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "[heap]", label)
	assert.Equal(t, data.Heap, labelType)
}

func TestExtractLabel_StackLabel(t *testing.T) {
	fields := []string{"7ffd00000000-7ffd00021000", "rw-p", "00000000", "00:00", "0", "[stack]"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "[stack]", label)
	assert.Equal(t, data.Stack, labelType)
}

func TestExtractLabel_AnonWithSpaces(t *testing.T) {
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "[anon:", "Go:", "gc", "bits]"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "[anon: Go: gc bits]", label)
	assert.Equal(t, data.Anonymous, labelType)
}

func TestExtractLabel_FilePath(t *testing.T) {
	fields := []string{"7f8a4c000000-7f8a4c021000", "r-xp", "00000000", "08:01", "12345", "/usr/lib/libc.so"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "/usr/lib/libc.so", label)
	assert.Equal(t, data.FileBacked, labelType)
}

func TestExtractLabel_AnonInode(t *testing.T) {
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "anon_inode:bpf-map"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "anon_inode:bpf-map", label)
	assert.Equal(t, data.Anonymous, labelType)
}

func TestExtractLabel_InodeNumber(t *testing.T) {
	// Remainder is just a number (inode) — should return empty
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "12345", "67890"}
	labelType, label := extractLabel(fields)
	assert.Equal(t, "67890", label)
	assert.Equal(t, data.Anonymous, labelType)
}

func TestParseKBToPages(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Rss:                  12 kB", 3},
		{"Swap:                  0 kB", 0},
		{"Rss:                 100 kB", 25},
		{"Rss: abc kB", 0},
		{"", 0},
		{"Rss:", 0},
	}

	for _, tc := range tests {
		result := parseKBToPages(tc.input)
		assert.Equal(t, tc.expected, result, "input: %q", tc.input)
	}
}
