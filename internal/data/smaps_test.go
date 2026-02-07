package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractLabel_Empty(t *testing.T) {
	fields := []string{"addr", "rw-p", "00000000", "00:00", "0"}
	assert.Equal(t, "", extractLabel(fields, "addr rw-p 00000000 00:00 0"))
}

func TestExtractLabel_TooFewFields(t *testing.T) {
	fields := []string{"addr", "rw-p"}
	assert.Equal(t, "", extractLabel(fields, "addr rw-p"))
}

func TestExtractLabel_BracketedLabel(t *testing.T) {
	line := "7f8a4c000000-7f8a4c021000 rw-p 00000000 00:00 0 [heap]"
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "[heap]"}
	assert.Equal(t, "[heap]", extractLabel(fields, line))
}

func TestExtractLabel_StackLabel(t *testing.T) {
	line := "7ffd00000000-7ffd00021000 rw-p 00000000 00:00 0 [stack]"
	fields := []string{"7ffd00000000-7ffd00021000", "rw-p", "00000000", "00:00", "0", "[stack]"}
	assert.Equal(t, "[stack]", extractLabel(fields, line))
}

func TestExtractLabel_AnonWithSpaces(t *testing.T) {
	line := "7f8a4c000000-7f8a4c021000 rw-p 00000000 00:00 0 [anon: Go: gc bits]"
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "[anon:", "Go:", "gc", "bits]"}
	assert.Equal(t, "[anon: Go: gc bits]", extractLabel(fields, line))
}

func TestExtractLabel_FilePath(t *testing.T) {
	line := "7f8a4c000000-7f8a4c021000 r-xp 00000000 08:01 12345 /usr/lib/libc.so"
	fields := []string{"7f8a4c000000-7f8a4c021000", "r-xp", "00000000", "08:01", "12345", "/usr/lib/libc.so"}
	assert.Equal(t, "/usr/lib/libc.so", extractLabel(fields, line))
}

func TestExtractLabel_AnonInode(t *testing.T) {
	line := "7f8a4c000000-7f8a4c021000 rw-p 00000000 00:00 0 anon_inode:bpf-map"
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "0", "anon_inode:bpf-map"}
	assert.Equal(t, "anon_inode:bpf-map", extractLabel(fields, line))
}

func TestExtractLabel_InodeNumber(t *testing.T) {
	// Remainder is just a number (inode) — should return empty
	line := "7f8a4c000000-7f8a4c021000 rw-p 00000000 00:00 12345 67890"
	fields := []string{"7f8a4c000000-7f8a4c021000", "rw-p", "00000000", "00:00", "12345", "67890"}
	assert.Equal(t, "", extractLabel(fields, line))
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
