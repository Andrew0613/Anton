package history

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	storeRelativePath = ".anton/history/receipts.jsonl"
	maxSummaryBytes   = 1200
)

type Receipt struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Timestamp   string            `json:"timestamp"`
	Source      Source            `json:"source"`
	Freshness   string            `json:"freshness"`
	Confidence  string            `json:"confidence"`
	ContentHash string            `json:"content_hash"`
	Summary     string            `json:"summary"`
	Payload     map[string]string `json:"payload,omitempty"`
}

type Source struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type Store struct {
	path string
}

type StoreReadResult struct {
	Receipts []Receipt
	Warnings []Warning
}

func NewStore(repoRoot string) Store {
	return Store{path: filepath.Join(repoRoot, storeRelativePath)}
}

func (store Store) Path() string {
	return store.path
}

func (store Store) Read() StoreReadResult {
	file, err := os.Open(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return StoreReadResult{Receipts: []Receipt{}}
	}
	if err != nil {
		return StoreReadResult{
			Receipts: []Receipt{},
			Warnings: []Warning{{
				Code:    "receipt-store-read-failed",
				Message: err.Error(),
				Path:    store.path,
			}},
		}
	}
	defer file.Close()

	var receipts []Receipt
	var warnings []Warning
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var receipt Receipt
		if err := json.Unmarshal([]byte(line), &receipt); err != nil {
			warnings = append(warnings, Warning{
				Code:    "malformed-receipt",
				Message: fmt.Sprintf("line %d is not a valid history receipt: %v", lineNumber, err),
				Path:    store.path,
			})
			continue
		}
		receipts = append(receipts, receipt)
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, Warning{
			Code:    "receipt-store-scan-failed",
			Message: err.Error(),
			Path:    store.path,
		})
	}

	sortReceipts(receipts)
	return StoreReadResult{Receipts: receipts, Warnings: warnings}
}

func (store Store) AppendNew(receipts []Receipt) (int, []Warning) {
	if len(receipts) == 0 {
		return 0, nil
	}

	existing := store.Read()
	known := make(map[string]bool, len(existing.Receipts))
	for _, receipt := range existing.Receipts {
		known[receipt.ID] = true
	}

	var toAppend []Receipt
	for _, receipt := range receipts {
		if known[receipt.ID] {
			continue
		}
		known[receipt.ID] = true
		toAppend = append(toAppend, receipt)
	}
	if len(toAppend) == 0 {
		return 0, existing.Warnings
	}

	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return 0, append(existing.Warnings, Warning{
			Code:    "receipt-store-create-failed",
			Message: err.Error(),
			Path:    filepath.Dir(store.path),
		})
	}

	file, err := os.OpenFile(store.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, append(existing.Warnings, Warning{
			Code:    "receipt-store-open-failed",
			Message: err.Error(),
			Path:    store.path,
		})
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, receipt := range toAppend {
		if err := encoder.Encode(receipt); err != nil {
			return 0, append(existing.Warnings, Warning{
				Code:    "receipt-store-write-failed",
				Message: err.Error(),
				Path:    store.path,
			})
		}
	}
	return len(toAppend), existing.Warnings
}

func newReceipt(receiptType string, source Source, timestamp time.Time, confidence string, content []byte, payload map[string]string) Receipt {
	contentHash := contentHash(content)
	summary := summarize(content)
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return Receipt{
		ID:          receiptID(receiptType, source, contentHash),
		Type:        receiptType,
		Timestamp:   timestamp.UTC().Format(time.RFC3339),
		Source:      source,
		Freshness:   freshness(timestamp),
		Confidence:  confidence,
		ContentHash: contentHash,
		Summary:     summary,
		Payload:     normalizePayload(payload),
	}
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func receiptID(receiptType string, source Source, contentHash string) string {
	stable := receiptType + "\x00" + source.Kind + "\x00" + filepath.ToSlash(source.Path) + "\x00" + contentHash
	sum := sha256.Sum256([]byte(stable))
	return "hist_" + hex.EncodeToString(sum[:])[:24]
}

func summarize(content []byte) string {
	value := redact(string(content))
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > maxSummaryBytes {
		value = value[:maxSummaryBytes] + "...[truncated]"
	}
	return value
}

func normalizePayload(payload map[string]string) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(payload))
	for key, value := range payload {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalized[key] = summarize([]byte(value))
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func freshness(timestamp time.Time) string {
	age := time.Since(timestamp)
	switch {
	case age < 0:
		return "future"
	case age <= 24*time.Hour:
		return "fresh"
	case age <= 14*24*time.Hour:
		return "recent"
	default:
		return "stale"
	}
}

func sortReceipts(receipts []Receipt) {
	sort.SliceStable(receipts, func(i, j int) bool {
		if receipts[i].Timestamp == receipts[j].Timestamp {
			return receipts[i].ID < receipts[j].ID
		}
		return receipts[i].Timestamp > receipts[j].Timestamp
	})
}
