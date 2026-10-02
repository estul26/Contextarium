// Package engine owns domain-neutral application services and validation.
package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxBody = 96 << 10

var (
	ErrInvalid     = errors.New("invalid argument")
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrValidation  = errors.New("schema validation failed")
	ErrUnavailable = errors.New("storage unavailable")
)

// ParseJSON rejects lossy/ambiguous JSON before any typed decoding or validation.
func ParseJSON(raw []byte) (any, error) {
	if len(raw) == 0 || len(raw) > MaxBody || !utf8.Valid(raw) || !validEscapes(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	nodes := 0
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		nodes++
		if depth > 32 || nodes > 4096 {
			return nil, ErrInvalid
		}
		token, err := d.Token()
		if err != nil {
			return nil, ErrInvalid
		}
		switch v := token.(type) {
		case json.Delim:
			switch v {
			case '{':
				obj := map[string]any{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return nil, ErrInvalid
					}
					name, ok := key.(string)
					if !ok {
						return nil, ErrInvalid
					}
					if _, exists := obj[name]; exists {
						return nil, ErrInvalid
					}
					value, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					obj[name] = value
				}
				end, err := d.Token()
				if err != nil || end != json.Delim('}') {
					return nil, ErrInvalid
				}
				return obj, nil
			case '[':
				arr := []any{}
				for d.More() {
					value, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					arr = append(arr, value)
				}
				end, err := d.Token()
				if err != nil || end != json.Delim(']') {
					return nil, ErrInvalid
				}
				return arr, nil
			default:
				return nil, ErrInvalid
			}
		case json.Number:
			if len(v.String()) > 128 {
				return nil, ErrInvalid
			}
			if i := strings.IndexAny(v.String(), "eE"); i >= 0 {
				exp, err := strconv.Atoi(v.String()[i+1:])
				if err != nil || exp < -308 || exp > 308 {
					return nil, ErrInvalid
				}
			}
		}
		return token, nil
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	return value, nil
}

// encoding/json otherwise replaces unpaired UTF-16 surrogates with U+FFFD.
func validEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func object(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	value, err := ParseJSON(raw)
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalid
	}
	valid := map[string]bool{}
	for _, name := range allowed {
		valid[name] = true
	}
	for name := range obj {
		if !valid[name] {
			return nil, ErrInvalid
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, ErrInvalid
	}
	return fields, nil
}

func required(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		v, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return false
		}
	}
	return true
}
