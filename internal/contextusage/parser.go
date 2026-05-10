// Package contextusage parses Claude Code transcript JSONL files to extract
// the most recent context usage information for statusline display.
//
// Parse is the sole public entry point. It NEVER returns an error — all failure
// modes (missing file, malformed lines, no usage data) return a zero Usage{}.
package contextusage

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
)

const (
	defaultLimit = 200_000

	// readTailBytes is the maximum number of bytes read from the end of the file.
	// 16 KB covers the last 15 lines comfortably for typical transcript line sizes.
	readTailBytes = 16 * 1024

	// maxLines is the maximum number of tail lines we inspect.
	maxLines = 15
)

var (
	reAutoCompact = regexp.MustCompile(`Context left until auto-compact: (\d+)%`)
	reContextLow  = regexp.MustCompile(`Context low \((\d+)% remaining\)`)
)

// Usage holds the parsed context usage data extracted from a transcript.
type Usage struct {
	Tokens  int     // total tokens (input + cache_read + cache_creation)
	Limit   int     // context window limit (default 200_000)
	Percent float64 // 0..100, computed from Tokens/Limit
	Method  string  // "usage" | "system" | ""
	Source  string  // "transcript" | ""
}

// assistantMessage is a minimal parse target for assistant JSONL entries.
type assistantMessage struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens               int `json:"input_tokens"`
			CacheReadInputTokens      int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens  int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// systemMessage is a minimal parse target for system_message JSONL entries.
type systemMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// rawType is used for a fast type-only peek before full decode.
type rawType struct {
	Type string `json:"type"`
}

// Parse reads the last maxLines lines of the transcript JSONL and extracts the
// most recent context usage. Returns zero Usage{} if the file is missing, empty,
// or contains no usable data. Never panics, never returns an error.
//
// limit is the context window size in tokens. If limit <= 0, it falls back to
// defaultLimit (200_000). Pass contextusage-specific model limits (e.g. 1_000_000
// for Opus 4.7 [1m]) so that Percent is computed correctly against the real window.
func Parse(transcriptPath string, limit int) Usage {
	if limit <= 0 {
		limit = defaultLimit
	}
	lines := readTailLines(transcriptPath)
	if len(lines) == 0 {
		return Usage{}
	}

	// Iterate in reverse: most recent line first.
	// assistant-usage wins over system_message if found first (i.e., more recent).
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if len(line) == 0 {
			continue
		}

		// Peek at type without full decode.
		var rt rawType
		if err := json.Unmarshal(line, &rt); err != nil {
			continue
		}

		switch rt.Type {
		case "assistant":
			var am assistantMessage
			if err := json.Unmarshal(line, &am); err != nil {
				continue
			}
			u := am.Message.Usage
			total := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
			if total > 0 {
				return usageFromTokens(total, limit)
			}

		case "system_message":
			var sm systemMessage
			if err := json.Unmarshal(line, &sm); err != nil {
				continue
			}
			if u, ok := parseSystemContent(sm.Content, limit); ok {
				return u
			}
		}
	}

	return Usage{}
}

// usageFromTokens builds a Usage with Method="usage" from a raw token count.
// limit is the effective context window (callers must pass > 0).
func usageFromTokens(tokens, limit int) Usage {
	pct := float64(tokens) / float64(limit) * 100.0
	if pct > 100.0 {
		pct = 100.0
	}
	return Usage{
		Tokens:  tokens,
		Limit:   limit,
		Percent: pct,
		Method:  "usage",
		Source:  "transcript",
	}
}

// parseSystemContent tries to extract a percent from system_message content.
// Returns (Usage, true) on match, (Usage{}, false) otherwise.
// limit is the effective context window (callers must pass > 0).
func parseSystemContent(content string, limit int) (Usage, bool) {
	// "Context left until auto-compact: X%"
	if m := reAutoCompact.FindStringSubmatch(content); len(m) == 2 {
		left, err := strconv.Atoi(m[1])
		if err != nil {
			return Usage{}, false
		}
		pct := float64(100 - left)
		return Usage{
			Percent: pct,
			Limit:   limit,
			Method:  "system",
			Source:  "transcript",
		}, true
	}

	// "Context low (X% remaining)"
	if m := reContextLow.FindStringSubmatch(content); len(m) == 2 {
		left, err := strconv.Atoi(m[1])
		if err != nil {
			return Usage{}, false
		}
		pct := float64(100 - left)
		return Usage{
			Percent: pct,
			Limit:   limit,
			Method:  "system",
			Source:  "transcript",
		}, true
	}

	return Usage{}, false
}

// readTailLines opens the file, seeks to the last readTailBytes, then returns
// up to maxLines raw JSON lines (as byte slices). Returns nil on any error.
func readTailLines(path string) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Seek to tail position.
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	size := info.Size()
	seekPos := size - readTailBytes
	if seekPos < 0 {
		seekPos = 0
	}
	if _, err := f.Seek(seekPos, io.SeekStart); err != nil {
		return nil
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer to handle long lines (transcript lines can be large).
	scanner.Buffer(make([]byte, readTailBytes), readTailBytes)

	var lines [][]byte
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}
		// Copy — scanner reuses its internal buffer.
		cp := make([]byte, len(b))
		copy(cp, b)
		lines = append(lines, cp)
	}

	// Keep only last maxLines.
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}
