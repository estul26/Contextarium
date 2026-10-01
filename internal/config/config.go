// Package config loads the local development runtime configuration without reading secrets.
package config

import (
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr string
	DBPath     string
	LogLevel   slog.Level
}

// Load treats explicitly empty environment variables as invalid, not as defaults.
func Load(lookup func(string) (string, bool)) (Config, error) {
	value := func(key, fallback string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return fallback
	}
	c := Config{
		ListenAddr: value("CONTEXTARIUM_LISTEN_ADDR", "127.0.0.1:8080"),
		DBPath:     value("CONTEXTARIUM_DB_PATH", "./data/contextarium.db"),
	}
	switch strings.ToLower(value("CONTEXTARIUM_LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return Config{}, errors.New("CONTEXTARIUM_LOG_LEVEL must be debug, info, warn, or error")
	}
	// Resolve this one convenience name deterministically, without DNS.
	if host, port, err := net.SplitHostPort(c.ListenAddr); err == nil && host == "localhost" {
		c.ListenAddr = net.JoinHostPort("127.0.0.1", port)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return errors.New("CONTEXTARIUM_LISTEN_ADDR must contain a loopback IP and port")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return errors.New("CONTEXTARIUM_LISTEN_ADDR must be loopback-only in M0-M2")
	}
	if port == "" || strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return errors.New("CONTEXTARIUM_LISTEN_ADDR port must be an integer from 0 to 65535")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return errors.New("CONTEXTARIUM_LISTEN_ADDR port must be an integer from 0 to 65535")
	}
	if strings.TrimSpace(c.DBPath) == "" || strings.ContainsRune(c.DBPath, 0) ||
		strings.HasPrefix(c.DBPath, "file:") || c.DBPath == ":memory:" {
		return errors.New("CONTEXTARIUM_DB_PATH must be a nonempty filesystem path, not a SQLite URI or in-memory database")
	}
	switch c.LogLevel {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
	default:
		return errors.New("unsupported log level")
	}
	return nil
}
