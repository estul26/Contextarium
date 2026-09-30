package config

import (
	"log/slog"
	"strings"
	"testing"
)

func fromMap(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}

func TestDefaults(t *testing.T) {
	c, err := Load(fromMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != "127.0.0.1:8080" || c.DBPath != "./data/contextarium.db" || c.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestExplicitConfiguration(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "127.0.0.2:65535", "[::1]:8081", "localhost:8080"} {
		t.Run(address, func(t *testing.T) {
			c, err := Load(fromMap(map[string]string{
				"CONTEXTARIUM_LISTEN_ADDR": address,
				"CONTEXTARIUM_DB_PATH":     "./synthetic folder/test?name.db",
				"CONTEXTARIUM_LOG_LEVEL":   "DEBUG",
			}))
			if err != nil {
				t.Fatal(err)
			}
			if c.LogLevel != slog.LevelDebug || c.DBPath != "./synthetic folder/test?name.db" {
				t.Fatalf("configuration not preserved: %+v", c)
			}
			if address == "localhost:8080" && c.ListenAddr != "127.0.0.1:8080" {
				t.Fatal("localhost was not fixed to a numeric loopback address")
			}
		})
	}
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if _, err := Load(fromMap(map[string]string{"CONTEXTARIUM_LOG_LEVEL": level})); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInvalidConfiguration(t *testing.T) {
	cases := []struct{ key, value string }{
		{"CONTEXTARIUM_LISTEN_ADDR", ""},
		{"CONTEXTARIUM_LISTEN_ADDR", ":8080"},
		{"CONTEXTARIUM_LISTEN_ADDR", "0.0.0.0:8080"},
		{"CONTEXTARIUM_LISTEN_ADDR", "[::]:8080"},
		{"CONTEXTARIUM_LISTEN_ADDR", "192.0.2.1:8080"},
		{"CONTEXTARIUM_LISTEN_ADDR", "example.com:8080"},
		{"CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1"},
		{"CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1:-1"},
		{"CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1:+80"},
		{"CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1:65536"},
		{"CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1:http"},
		{"CONTEXTARIUM_LISTEN_ADDR", "[::1%lo0]:8080"},
		{"CONTEXTARIUM_DB_PATH", ""},
		{"CONTEXTARIUM_DB_PATH", " "},
		{"CONTEXTARIUM_DB_PATH", "file:test.db?mode=memory"},
		{"CONTEXTARIUM_DB_PATH", ":memory:"},
		{"CONTEXTARIUM_DB_PATH", "bad\x00path"},
		{"CONTEXTARIUM_LOG_LEVEL", ""},
		{"CONTEXTARIUM_LOG_LEVEL", "SYNTHETIC_SECRET_INVALID_LEVEL"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			_, err := Load(fromMap(map[string]string{tc.key: tc.value}))
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "SYNTHETIC_SECRET") {
				t.Fatal("untrusted configuration value leaked in error")
			}
		})
	}
}
