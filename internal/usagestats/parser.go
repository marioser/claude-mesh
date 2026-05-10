package usagestats

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// assistantEntry is a minimal parse target for assistant-type JSONL lines.
type assistantEntry struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	Message   struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// rawType peeks at the "type" field of a JSONL line.
type rawType struct {
	Type string `json:"type"`
}

// ScanProjects walks projectsDir (typically ~/.claude/projects/), parses all *.jsonl files,
// and returns token-usage entries for assistant messages. If since is non-zero, only entries
// at or after that time are returned.
//
// Errors on individual files are logged but do not fail the whole scan.
// Returns a non-nil error only if projectsDir cannot be read.
func ScanProjects(projectsDir string, since time.Time) ([]Entry, error) {
	// Read the top-level directory (each subdir = one project).
	topEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("usagestats.ScanProjects: read dir %s: %w", projectsDir, err)
	}

	var results []Entry
	for _, proj := range topEntries {
		if !proj.IsDir() {
			continue
		}
		projDir := filepath.Join(projectsDir, proj.Name())
		files, err := os.ReadDir(projDir)
		if err != nil {
			log.Printf("usagestats: skip project %s: %v", proj.Name(), err)
			continue
		}
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".jsonl" {
				continue
			}
			path := filepath.Join(projDir, f.Name())
			entries, err := parseJSONL(path, since)
			if err != nil {
				log.Printf("usagestats: skip file %s: %v", path, err)
				continue
			}
			results = append(results, entries...)
		}
	}
	return results, nil
}

// parseJSONL reads a single JSONL file and extracts token-usage entries.
func parseJSONL(path string, since time.Time) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	var results []Entry
	scanner := bufio.NewScanner(f)
	// Allow large lines (transcript lines can be several KB).
	const maxLineBytes = 256 * 1024
	scanner.Buffer(make([]byte, maxLineBytes), maxLineBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Fast peek at type.
		var rt rawType
		if err := json.Unmarshal(line, &rt); err != nil {
			continue
		}
		if rt.Type != "assistant" {
			continue
		}

		// Full decode.
		var ae assistantEntry
		if err := json.Unmarshal(line, &ae); err != nil {
			continue
		}

		tokens := ae.Message.Usage.InputTokens +
			ae.Message.Usage.CacheCreationInputTokens +
			ae.Message.Usage.CacheReadInputTokens
		if tokens <= 0 {
			continue
		}
		if !since.IsZero() && ae.Timestamp.Before(since) {
			continue
		}

		results = append(results, Entry{
			SessionID: ae.SessionID,
			Timestamp: ae.Timestamp,
			Tokens:    tokens,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return results, nil
}
