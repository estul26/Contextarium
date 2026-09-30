package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationFailureDoesNotLogValues(t *testing.T) {
	var logs bytes.Buffer
	lookup := func(key string) (string, bool) {
		if key == "CONTEXTARIUM_LOG_LEVEL" {
			return "SYNTHETIC_SECRET", true
		}
		return "", false
	}
	if code := run(context.Background(), &logs, lookup); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(logs.String(), "SYNTHETIC_SECRET") {
		t.Fatal("configuration value leaked")
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil || entry["level"] != "ERROR" {
		t.Fatalf("not structured error logging: %s", logs.String())
	}
}

func TestDatabaseFailureDoesNotLogPathOrContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SYNTHETIC_PRIVATE_NAME.db")
	if err := os.WriteFile(path, []byte("SYNTHETIC_PRIVATE_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	lookup := func(key string) (string, bool) {
		if key == "CONTEXTARIUM_DB_PATH" {
			return path, true
		}
		return "", false
	}
	if code := run(context.Background(), &logs, lookup); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(logs.String(), "SYNTHETIC_PRIVATE") {
		t.Fatal("database path or contents leaked")
	}
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
	}
}
