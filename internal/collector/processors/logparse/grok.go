package logparse

import (
	"fmt"
	"regexp"
	"strings"
)

// builtinPatterns defines the default grok pattern library.
var builtinPatterns = map[string]string{
	"IPORHOST":          `(?:%{IP}|%{HOSTNAME})`,
	"IP":                `(?:%{IPV4}|%{IPV6})`,
	"IPV4":              `(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`,
	"IPV6":              `(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|::(?:[0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}|[0-9a-fA-F]{1,4}::(?:[0-9a-fA-F]{1,4}:){0,4}[0-9a-fA-F]{1,4}`,
	"HOSTNAME":          `\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`,
	"NUMBER":            `(?:%{BASE10NUM})`,
	"BASE10NUM":         `(?:[+-]?(?:[0-9]+(?:\.[0-9]+)?)|[+-]?\.[0-9]+)`,
	"DATA":              `.*?`,
	"GREEDYDATA":        `.*`,
	"TIMESTAMP_ISO8601": `\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`,
	"LOGLEVEL":          `(?:EMERG|ALERT|CRIT(?:ICAL)?|ERR(?:OR)?|WARN(?:ING)?|NOTICE|INFO|DEBUG|FATAL|PANIC)`,
	"WORD":              `\b\w+\b`,
	"QUOTEDSTRING":      `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`,
	"UUID":              `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`,
}

// Grok holds a compiled grok pattern with named capture groups.
type Grok struct {
	re     *regexp.Regexp
	groups []string // ordered list of named groups
}

// NewGrok compiles a grok pattern string into a Grok.
//
// Patterns use %{PATTERN_NAME} for non-capturing groups and
// %{PATTERN_NAME:capture_name} for named capture groups.
// Custom patterns extend or override the built-in pattern library.
func NewGrok(pattern string, customPatterns map[string]string) (*Grok, error) {
	// Merge custom patterns over built-in ones.
	all := make(map[string]string, len(builtinPatterns)+len(customPatterns))
	for k, v := range builtinPatterns {
		all[k] = v
	}
	for k, v := range customPatterns {
		all[k] = v
	}

	// Expand the top-level pattern, then expand any nested references.
	regex, names, err := expandPattern(pattern, all)
	if err != nil {
		return nil, err
	}

	re, err := regexp.Compile("^(?:" + regex + ")$")
	if err != nil {
		return nil, fmt.Errorf("logparse: compiled grok regex is invalid: %w", err)
	}

	return &Grok{re: re, groups: names}, nil
}

// Match applies the grok pattern to the input string and returns named captures.
// Returns an error if the pattern does not match.
func (g *Grok) Match(input string) (map[string]string, error) {
	match := g.re.FindStringSubmatch(input)
	if match == nil {
		return nil, fmt.Errorf("logparse: pattern did not match input %q", input)
	}

	names := g.re.SubexpNames()
	result := make(map[string]string, len(g.groups))
	for i, name := range names {
		if i > 0 && i < len(match) && name != "" {
			result[name] = match[i]
		}
	}
	return result, nil
}

// expandPattern takes a grok pattern string and a pattern library, and returns
// the expanded regex and the list of named capture groups.
func expandPattern(pattern string, library map[string]string) (string, []string, error) {
	var names []string

	// We repeatedly expand until no more %{...} placeholders remain.
	// Limit iterations to prevent infinite loops from circular references.
	const maxIterations = 10
	for range maxIterations {
		if !strings.Contains(pattern, "%{") {
			break
		}

		var err error
		pattern, names, err = expandOnce(pattern, library, names)
		if err != nil {
			return "", nil, err
		}
	}

	return pattern, names, nil
}

// expandOnce performs a single pass of %{...} expansion.
func expandOnce(pattern string, library map[string]string, existingNames []string) (string, []string, error) {
	names := existingNames
	var result strings.Builder
	i := 0

	for i < len(pattern) {
		if i+1 < len(pattern) && pattern[i] == '%' && pattern[i+1] == '{' {
			// Find the closing brace.
			end := strings.IndexByte(pattern[i+2:], '}')
			if end < 0 {
				return "", nil, fmt.Errorf("logparse: unmatched %%{ in pattern at position %d", i)
			}
			end += i + 2

			inner := pattern[i+2 : end]

			// Split on ':' to get pattern name and optional capture name.
			var patternName, captureName string
			if idx := strings.IndexByte(inner, ':'); idx >= 0 {
				patternName = inner[:idx]
				captureName = inner[idx+1:]
			} else {
				patternName = inner
			}

			defn, ok := library[patternName]
			if !ok {
				return "", nil, fmt.Errorf("logparse: unknown grok pattern %q", patternName)
			}

			if captureName != "" {
				// Named capture group: (?P<name>...)
				fmt.Fprintf(&result, "(?P<%s>%s)", captureName, defn)
				names = append(names, captureName)
			} else {
				// Non-capturing group: (?:...)
				fmt.Fprintf(&result, "(?:%s)", defn)
			}

			i = end + 1
		} else {
			result.WriteByte(pattern[i])
			i++
		}
	}

	return result.String(), names, nil
}
