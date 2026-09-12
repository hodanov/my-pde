package permission

import (
	"bufio"
	"encoding/json"
	"io"
	"net/url"
	"slices"
	"strings"

	"agent-stats/internal/parser"
)

// RuleSet is a set of permission rules compared in normalized form.
type RuleSet map[string]struct{}

// NewRuleSet collects every rule of the given lists.
func NewRuleSet(lists ...[]string) RuleSet {
	set := RuleSet{}
	for _, list := range lists {
		for _, rule := range list {
			set[normalizeRule(rule)] = struct{}{}
		}
	}
	return set
}

// Contains reports whether rule, once normalized, is in the set.
func (s RuleSet) Contains(rule string) bool {
	_, ok := s[normalizeRule(rule)]
	return ok
}

type settingsFile struct {
	Permissions struct {
		Allow []string `json:"allow"`
		Ask   []string `json:"ask"`
		Deny  []string `json:"deny"`
	} `json:"permissions"`
}

func decodeSettings(r io.Reader) (settingsFile, error) {
	var s settingsFile
	err := json.NewDecoder(r).Decode(&s)
	return s, err
}

// SettingsRules returns every allow, ask and deny rule of a Claude Code
// settings file: a rule in any of them is already a deliberate decision.
func SettingsRules(r io.Reader) ([]string, error) {
	s, err := decodeSettings(r)
	if err != nil {
		return nil, err
	}
	p := s.Permissions
	return slices.Concat(p.Allow, p.Ask, p.Deny), nil
}

// LocalAllowRules returns the allow rules of a repository's
// .claude/settings.local.json.
func LocalAllowRules(r io.Reader) ([]string, error) {
	s, err := decodeSettings(r)
	if err != nil {
		return nil, err
	}
	return s.Permissions.Allow, nil
}

// ReadDeclined returns the rules previously declined in review, one per line.
func ReadDeclined(r io.Reader) ([]string, error) {
	var rules []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if rule := strings.TrimSpace(sc.Text()); rule != "" {
			rules = append(rules, rule)
		}
	}
	return rules, sc.Err()
}

func normalizeRule(rule string) string {
	if base, ok := strings.CutSuffix(rule, ":*)"); ok {
		return base + " *)"
	}
	return rule
}

func ruleFamily(rule string) string {
	tool, content, hasContent := strings.Cut(rule, "(")
	if !hasContent {
		return tool
	}
	content = strings.TrimSuffix(content, ")")
	switch tool {
	case "Bash":
		if lead, _ := parser.LeadingCommand(content); lead != "" {
			return "Bash:" + lead
		}
	case "WebFetch":
		if domain, ok := strings.CutPrefix(content, "domain:"); ok {
			return "WebFetch:" + domain
		}
	}
	return ""
}

func callFamily(call *parser.ToolCall) string {
	switch {
	case call.Name == "Bash":
		if lead, _ := parser.LeadingCommand(inputField(call.Input, "command")); lead != "" {
			return "Bash:" + lead
		}
	case call.Name == "WebFetch":
		if host := hostOf(inputField(call.Input, "url")); host != "" {
			return "WebFetch:" + host
		}
	case strings.HasPrefix(call.Name, "mcp__"):
		return call.Name
	}
	return ""
}

func draftRule(call *parser.ToolCall) string {
	switch call.Name {
	case "Bash":
		if lead, _ := parser.LeadingCommand(inputField(call.Input, "command")); lead != "" {
			return "Bash(" + lead + " *)"
		}
	case "WebFetch":
		if host := hostOf(inputField(call.Input, "url")); host != "" {
			return "WebFetch(domain:" + host + ")"
		}
	default:
		if path := inputField(call.Input, "file_path"); strings.HasPrefix(path, "/") {
			return call.Name + "(/" + path + ")"
		}
	}
	return call.Name
}

func summarizeCall(call *parser.ToolCall) string {
	const maxRunes = 120
	var text string
	switch call.Name {
	case "Bash":
		text, _, _ = strings.Cut(inputField(call.Input, "command"), "\n")
	case "WebFetch":
		text = inputField(call.Input, "url")
	default:
		text = inputField(call.Input, "file_path")
		if text == "" {
			text = string(call.Input)
		}
	}
	if runes := []rune(text); len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return text
}

func inputField(input json.RawMessage, key string) string {
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	s, _ := fields[key].(string)
	return s
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
