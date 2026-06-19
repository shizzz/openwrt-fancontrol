// Package config — UCI parser.
//
// OpenWrt's Unified Configuration Interface (UCI) is a flat INI-like
// format.  This file implements just enough of UCI to read a list of
// named sections from /etc/config/fancontrol; the daemon never writes
// UCI files itself.
//
// Supported syntax:
//
//	# comment
//	config <type> ['<name>']
//	    option <key> '<value>'
//
// Not supported (intentionally): anonymous sections, `list` directives,
// section inheritance, uci package metadata, multi-line values.  Add
// only when a real use case appears.
package config

import (
	"fmt"
	"os"
	"strings"
)

// defaultUCIPath is the conventional location for the daemon's UCI file.
const defaultUCIPath = "/etc/config/fancontrol"

// UCISection is one section parsed from a UCI file.
type UCISection struct {
	Type    string            // e.g. "fancontrol"
	Name    string            // e.g. "cpu"; empty for anonymous sections
	Options map[string]string // keyed by option name
}

// parseUCISections reads path and returns every section it finds.  A
// missing file yields a nil slice and no error; any other read failure
// is returned to the caller.
//
// Duplicate section names produce multiple entries; the caller is
// responsible for handling that (the daemon treats them as separate
// fans, each driven independently).
func parseUCISections(path string) ([]UCISection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read uci %q: %w", path, err)
	}

	var sections []UCISection

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "config ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			sec := UCISection{Type: fields[1], Options: make(map[string]string)}
			if len(fields) >= 3 {
				sec.Name = strings.Trim(fields[2], "'\"")
			}
			sections = append(sections, sec)
			continue
		}

		if len(sections) == 0 || !strings.HasPrefix(line, "option ") {
			continue
		}

		key, value, ok := parseOptionLine(line)
		if ok {
			sections[len(sections)-1].Options[key] = value
		}
	}

	return sections, nil
}

// parseOptionLine extracts key and value from "option <key> <value>".
// Values may be single- or double-quoted; quotes are stripped.  Unquoted
// values are taken verbatim (whitespace-separated).
func parseOptionLine(line string) (key, value string, ok bool) {
	rest := strings.TrimPrefix(line, "option ")
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return "", "", false
	}
	value = strings.Trim(strings.Join(fields[1:], " "), "'\"")
	return fields[0], value, true
}