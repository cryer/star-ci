// Package config loads the optional .star-ci.yml override file.
//
// Supported YAML subset (parsed by line scanning, no YAML library):
//
//	confidence: 0.7          # override profile.MinConfidence
//	coverage: 80             # required line-coverage percentage (0-100) for run
//	disable:                 # step IDs to remove from the plan
//	  - go-lint
//	append:                  # custom steps to append
//	  - id: docs-link-check
//	    name: Docs link check
//	    category: test       # install|lint|typecheck|test|build|security
//	    commands:
//	      - npx markdown-link-check README.md
//	    reason: declared in .star-ci.yml
//	    optional: true
//
// Rules: indentation uses spaces only, blank lines and full-line '#'
// comments are ignored, inline comments and flow syntax are not supported.
// A missing category defaults to test; an unknown category is an error.
// Appended steps must have an id and at least one command. Malformed input
// yields an error carrying the 1-based line number.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cryer/star-ci/internal/plan"
)

// FileName is the config file looked up at the repository root.
const FileName = ".star-ci.yml"

// Config is the parsed .star-ci.yml. The zero value means "no config".
type Config struct {
	Confidence *float64    // nil = keep profile.MinConfidence default
	Coverage   *float64    // nil = no coverage threshold; else 0-100 percent
	Disable    []string    // step IDs to drop from the plan
	Append     []plan.Step // custom steps appended to the plan
	Path       string      // file the config was loaded from, "" if none
}

// Loaded reports whether a config file was found.
func (c *Config) Loaded() bool { return c.Path != "" }

// Load reads root/.star-ci.yml. A missing file is not an error and yields
// the zero Config.
func Load(root string) (*Config, error) {
	cfg := &Config{}
	path := filepath.Join(root, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	cfg.Path = path
	if err := cfg.parse(string(data)); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Apply drops disabled steps by ID, appends custom steps, and re-sorts.
func (c *Config) Apply(p *plan.Plan) {
	if len(c.Disable) > 0 {
		disabled := make(map[string]bool, len(c.Disable))
		for _, id := range c.Disable {
			disabled[id] = true
		}
		kept := p.Steps[:0]
		for _, s := range p.Steps {
			if !disabled[s.ID] {
				kept = append(kept, s)
			}
		}
		p.Steps = kept
	}
	p.Steps = append(p.Steps, c.Append...)
	p.Sort()
}

func (c *Config) parse(text string) error {
	p := &parser{cfg: c, commandsIndent: -1}
	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		if err := p.line(raw, i+1); err != nil {
			return err
		}
	}
	return p.closeItem(len(lines))
}

type parser struct {
	cfg *Config

	section        string // "", "disable", "append"
	cur            *plan.Step
	curLine        int
	itemIndent     int
	commandsIndent int // indent of the open "commands:" key, -1 when closed
}

func (p *parser) line(raw string, no int) error {
	raw = strings.TrimSuffix(raw, "\r")
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	indent := 0
	for indent < len(raw) && raw[indent] == ' ' {
		indent++
	}
	if indent < len(raw) && raw[indent] == '\t' {
		return p.err(no, "tabs are not allowed for indentation")
	}

	if indent == 0 {
		if err := p.closeItem(no); err != nil {
			return err
		}
		p.commandsIndent = -1
		key, val, found := splitKey(trimmed)
		switch key {
		case "confidence":
			if !found || val == "" {
				return p.err(no, "confidence expects a numeric value")
			}
			f, err := strconv.ParseFloat(unquote(val), 64)
			if err != nil {
				return p.err(no, "invalid confidence %q: not a number", val)
			}
			if f < 0 || f > 1 {
				return p.err(no, "confidence %.2f out of range [0, 1]", f)
			}
			p.cfg.Confidence = &f
			p.section = ""
		case "coverage":
			if !found || val == "" {
				return p.err(no, "coverage expects a numeric value")
			}
			f, err := strconv.ParseFloat(unquote(val), 64)
			if err != nil {
				return p.err(no, "invalid coverage %q: not a number", val)
			}
			if f < 0 || f > 100 {
				return p.err(no, "coverage %.2f out of range [0, 100]", f)
			}
			p.cfg.Coverage = &f
			p.section = ""
		case "disable":
			if val != "" {
				return p.err(no, "disable expects a list on the following lines")
			}
			p.section = "disable"
		case "append":
			if val != "" {
				return p.err(no, "append expects a list on the following lines")
			}
			p.section = "append"
		default:
			return p.err(no, "unknown top-level key %q (want confidence|coverage|disable|append)", key)
		}
		return nil
	}

	switch p.section {
	case "disable":
		if !isListItem(trimmed) {
			return p.err(no, "disable expects list items like '- go-lint'")
		}
		id := unquote(strings.TrimSpace(trimmed[1:]))
		if id == "" {
			return p.err(no, "empty step id in disable list")
		}
		p.cfg.Disable = append(p.cfg.Disable, id)
		return nil
	case "append":
		return p.appendLine(trimmed, indent, no)
	default:
		return p.err(no, "unexpected indented line without a disable:/append: section")
	}
}

func (p *parser) appendLine(trimmed string, indent, no int) error {
	// Command entries belong to the open "commands:" key and sit deeper.
	if p.commandsIndent >= 0 && indent > p.commandsIndent && isListItem(trimmed) {
		cmd := unquote(strings.TrimSpace(trimmed[1:]))
		if cmd == "" {
			return p.err(no, "empty command in commands list")
		}
		p.cur.Commands = append(p.cur.Commands, cmd)
		return nil
	}
	p.commandsIndent = -1

	if isListItem(trimmed) {
		if err := p.closeItem(no); err != nil {
			return err
		}
		rest := strings.TrimSpace(trimmed[1:])
		if rest == "" {
			return p.err(no, "append item must start with a field, e.g. '- id: my-step'")
		}
		p.cur = &plan.Step{Category: plan.CatTest}
		p.curLine = no
		p.itemIndent = indent
		return p.setField(rest, indent, no)
	}
	if p.cur == nil {
		return p.err(no, "append expects items starting with '- id: <step-id>'")
	}
	if indent <= p.itemIndent {
		return p.err(no, "step fields must be indented under their '- id:' item")
	}
	return p.setField(trimmed, indent, no)
}

func (p *parser) setField(s string, indent, no int) error {
	key, val, found := splitKey(s)
	if !found {
		return p.err(no, "expected 'field: value' in append item")
	}
	val = unquote(val)
	switch key {
	case "id":
		if val == "" {
			return p.err(no, "step id must not be empty")
		}
		p.cur.ID = val
	case "name":
		p.cur.Name = val
	case "category":
		if val == "" {
			return p.err(no, "category must not be empty")
		}
		cat := plan.Category(val)
		if !validCategory(cat) {
			return p.err(no, "unknown category %q (want install|lint|typecheck|test|build|security)", val)
		}
		p.cur.Category = cat
	case "reason":
		p.cur.Reason = val
	case "optional":
		switch val {
		case "true":
			p.cur.Optional = true
		case "false":
			p.cur.Optional = false
		default:
			return p.err(no, "optional must be true or false, got %q", val)
		}
	case "commands":
		if val != "" {
			return p.err(no, "commands expects a list on the following lines")
		}
		p.commandsIndent = indent
	default:
		return p.err(no, "unknown step field %q (want id|name|category|commands|reason|optional)", key)
	}
	return nil
}

// closeItem validates and stores the step being built, if any.
func (p *parser) closeItem(_ int) error {
	if p.cur == nil {
		return nil
	}
	if p.cur.ID == "" {
		return p.err(p.curLine, "append item missing required 'id'")
	}
	if len(p.cur.Commands) == 0 {
		return p.err(p.curLine, "append item %q has no commands", p.cur.ID)
	}
	if p.cur.Name == "" {
		p.cur.Name = p.cur.ID
	}
	if p.cur.Reason == "" {
		p.cur.Reason = "declared in " + FileName
	}
	p.cfg.Append = append(p.cfg.Append, *p.cur)
	p.cur = nil
	return nil
}

func (p *parser) err(no int, format string, a ...any) error {
	return fmt.Errorf("%s:%d: %s", FileName, no, fmt.Sprintf(format, a...))
}

func splitKey(s string) (key, val string, found bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return strings.TrimSpace(s), "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}

func isListItem(trimmed string) bool {
	return trimmed == "-" || strings.HasPrefix(trimmed, "- ")
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func validCategory(c plan.Category) bool {
	for _, o := range plan.CategoryOrder {
		if c == o {
			return true
		}
	}
	return false
}
