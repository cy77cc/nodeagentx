package logparse

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/cy77cc/opsagent/internal/collector"
)

// ParseRule defines a single log parsing rule.
type ParseRule struct {
	// Field is the metric field name containing the raw text to parse.
	Field string `mapstructure:"field"`
	// Parser is the parsing strategy: "grok", "regex", or "json".
	Parser string `mapstructure:"parser"`
	// GrokPattern is the grok pattern (used when Parser is "grok").
	GrokPattern string `mapstructure:"grok_pattern"`
	// RegexPattern is the regex pattern with named groups (used when Parser is "regex").
	RegexPattern string `mapstructure:"regex_pattern"`
	// Patterns is a list of grok patterns to try in order (used when Parser is "grok").
	Patterns []string `mapstructure:"patterns"`

	// compiled holds the compiled primary grok (for "grok" parser).
	compiled *Grok
	// groks holds all pre-compiled grok patterns (for "grok" parser).
	groks []*Grok
	// compiledRegex holds the compiled regex (for "regex" parser).
	compiledRegex *regexp.Regexp
}

// LogParseProcessor applies parsing rules to metric fields.
type LogParseProcessor struct {
	Rules []ParseRule
}

// Init parses configuration from a map and compiles patterns.
func (p *LogParseProcessor) Init(cfg map[string]interface{}) error {
	raw, ok := cfg["rules"]
	if !ok {
		return nil
	}
	ruleList, ok := raw.([]interface{})
	if !ok {
		return fmt.Errorf("logparse: \"rules\" must be a list, got %T", raw)
	}

	rules := make([]ParseRule, 0, len(ruleList))
	for i, entry := range ruleList {
		ruleMap, ok := entry.(map[string]interface{})
		if !ok {
			return fmt.Errorf("logparse: rule entry %d must be a map, got %T", i, entry)
		}

		rule := ParseRule{}
		if v, ok := ruleMap["field"].(string); ok {
			rule.Field = v
		}
		if v, ok := ruleMap["parser"].(string); ok {
			rule.Parser = v
		}
		if v, ok := ruleMap["grok_pattern"].(string); ok {
			rule.GrokPattern = v
		}
		if v, ok := ruleMap["regex_pattern"].(string); ok {
			rule.RegexPattern = v
		}
		if v, ok := ruleMap["patterns"].([]interface{}); ok {
			for _, p := range v {
				if s, ok := p.(string); ok {
					rule.Patterns = append(rule.Patterns, s)
				}
			}
		}

		if rule.Field == "" {
			return fmt.Errorf("logparse: rule entry %d: field must not be empty", i)
		}
		if rule.Parser == "" {
			return fmt.Errorf("logparse: rule entry %d: parser must not be empty", i)
		}

		// Compile patterns based on parser type.
		switch rule.Parser {
		case "grok":
			patterns := rule.Patterns
			if rule.GrokPattern != "" {
				patterns = append([]string{rule.GrokPattern}, patterns...)
			}
			if len(patterns) == 0 {
				return fmt.Errorf("logparse: rule entry %d: grok parser requires grok_pattern or patterns", i)
			}
			// Pre-compile all grok patterns.
			groks := make([]*Grok, 0, len(patterns))
			for j, pat := range patterns {
				g, err := NewGrok(pat, nil)
				if err != nil {
					return fmt.Errorf("logparse: rule entry %d, pattern %d: %w", i, j, err)
				}
				groks = append(groks, g)
			}
			rule.compiled = groks[0]
			rule.groks = groks
			rule.Patterns = patterns

		case "regex":
			if rule.RegexPattern == "" {
				return fmt.Errorf("logparse: rule entry %d: regex parser requires regex_pattern", i)
			}
			re, err := regexp.Compile(rule.RegexPattern)
			if err != nil {
				return fmt.Errorf("logparse: rule entry %d: invalid regex pattern %q: %w", i, rule.RegexPattern, err)
			}
			rule.compiledRegex = re

		case "json":
			// No pre-compilation needed for JSON.

		default:
			return fmt.Errorf("logparse: rule entry %d: unknown parser %q (supported: grok, regex, json)", i, rule.Parser)
		}

		rules = append(rules, rule)
	}
	p.Rules = rules
	return nil
}

// Apply applies parsing rules to metrics, extracting fields from raw text.
func (p *LogParseProcessor) Apply(in []*collector.Metric) []*collector.Metric {
	for _, m := range in {
		fields := m.Fields()
		for _, rule := range p.Rules {
			rawVal, ok := fields[rule.Field]
			if !ok {
				continue
			}
			rawStr, ok := rawVal.(string)
			if !ok {
				continue
			}

			switch rule.Parser {
			case "grok":
				p.applyGrok(m, rule, rawStr)
			case "regex":
				p.applyRegex(m, rule, rawStr)
			case "json":
				p.applyJSON(m, rawStr)
			}
			// Refresh the fields snapshot so subsequent rules see
			// fields added by earlier rules.
			fields = m.Fields()
		}
	}
	return in
}

// applyGrok applies a grok rule to extract fields.
func (p *LogParseProcessor) applyGrok(m *collector.Metric, rule ParseRule, input string) {
	// Try pre-compiled patterns in order; use the first match.
	for _, g := range rule.groks {
		matches, err := g.Match(input)
		if err != nil {
			continue
		}
		for k, v := range matches {
			m.AddField(k, v)
		}
		return
	}
}

// applyRegex applies a regex rule to extract named capture groups as fields.
func (p *LogParseProcessor) applyRegex(m *collector.Metric, rule ParseRule, input string) {
	match := rule.compiledRegex.FindStringSubmatch(input)
	if match == nil {
		return
	}
	names := rule.compiledRegex.SubexpNames()
	for i, name := range names {
		if i > 0 && i < len(match) && name != "" {
			m.AddField(name, match[i])
		}
	}
}

// applyJSON parses the input as JSON and adds each top-level key as a field.
func (p *LogParseProcessor) applyJSON(m *collector.Metric, input string) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return
	}
	for k, v := range parsed {
		m.AddField(k, v)
	}
}

// SampleConfig returns a sample configuration string.
func (p *LogParseProcessor) SampleConfig() string {
	return `
[[rules]]
  field = "message"
  parser = "grok"
  grok_pattern = "%{IPORHOST:client} %{WORD:method} %{URIPATHPARAM:request} %{NUMBER:bytes} %{NUMBER:duration}"

[[rules]]
  field = "message"
  parser = "json"

[[rules]]
  field = "message"
  parser = "regex"
  regex_pattern = "^(?P<host>\\S+) (?P<message>.*)$"
`
}

func init() {
	collector.RegisterProcessor("logparse", func() collector.Processor {
		return &LogParseProcessor{}
	})
}
