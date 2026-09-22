package runner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cryer/star-ci/internal/plan"
	"github.com/cryer/star-ci/internal/profile"
)

// DigestLines is how many trailing lines of step output a failure digest keeps.
const DigestLines = 20

// Status is the outcome of one executed (or skipped) step.
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusWarned  Status = "warned"  // optional step that failed
	StatusSkipped Status = "skipped" // never ran because of fail-fast
)

// StepResult records what happened to one step of a plan run.
type StepResult struct {
	Step   plan.Step
	Status Status
	Output string // captured stdout+stderr of the step's commands
}

// Digest returns the tail of output: at most n lines, with a marker noting
// how many earlier lines were omitted. Trailing blank lines are dropped.
func Digest(output string, n int) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	omitted := len(lines) - n
	tail := strings.Join(lines[omitted:], "\n")
	return fmt.Sprintf("... (%d earlier lines omitted)\n%s", omitted, tail)
}

// RenderSummary builds the Markdown job summary for a finished run. cov is
// the coverage check outcome, nil when no threshold was enforced.
func RenderSummary(pl plan.Plan, results []StepResult, cov *CoverageResult) string {
	var b strings.Builder
	b.WriteString("## star-ci summary\n\n")
	renderProfile(&b, pl.Profile)
	b.WriteString("\n### Steps\n\n")
	b.WriteString("| Step | Category | Status | Reason |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			escapeCell(r.Step.Name), escapeCell(string(r.Step.Category)),
			r.Status, escapeCell(r.Step.Reason))
	}
	if cov != nil {
		b.WriteString("\n### Coverage\n\n")
		status := "passed"
		if !cov.Met() {
			status = "below threshold"
		}
		fmt.Fprintf(&b, "- **%.1f%%** from `%s` (threshold %.1f%% — %s)\n",
			cov.Percent, escapeCell(cov.Source), cov.Threshold, status)
	}
	var digests []StepResult
	for _, r := range results {
		if (r.Status == StatusFailed || r.Status == StatusWarned) && strings.TrimSpace(r.Output) != "" {
			digests = append(digests, r)
		}
	}
	if len(digests) > 0 {
		b.WriteString("\n### Failure digests\n\n")
		for _, r := range digests {
			fmt.Fprintf(&b, "<details>\n<summary>%s (%s) — %s</summary>\n\n```\n%s\n```\n\n</details>\n\n",
				escapeCell(r.Step.Name), r.Step.ID, r.Status, Digest(r.Output, DigestLines))
		}
	}
	return b.String()
}

func renderProfile(b *strings.Builder, p profile.Profile) {
	var langs []string
	for _, l := range p.Languages {
		if l.VersionHint != "" {
			langs = append(langs, l.Name+" "+l.VersionHint)
		} else {
			langs = append(langs, l.Name)
		}
	}
	if len(langs) == 0 {
		b.WriteString("**Detected:** no languages\n")
	} else {
		fmt.Fprintf(b, "**Detected:** %s\n", strings.Join(langs, ", "))
	}
	b.WriteString("\n")
	field := func(label, value string) {
		if value != "" {
			fmt.Fprintf(b, "- **%s:** %s\n", label, value)
		}
	}
	field("Package manager", p.PackageManager)
	field("Test runner", p.TestRunner)
	field("Typecheck", p.Typecheck)
	if len(p.Linters) > 0 {
		field("Linters", strings.Join(p.Linters, ", "))
	}
	if len(p.Formatters) > 0 {
		field("Formatters", strings.Join(p.Formatters, ", "))
	}
	if p.HasDockerfile {
		field("Dockerfile", "present")
	}
}

// escapeCell makes a value safe for a Markdown table cell.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// writeStepSummary appends the Markdown summary to the file named by the
// GITHUB_STEP_SUMMARY environment variable. Outside GitHub Actions (variable
// unset) it is a no-op. Write failures are reported but never fail the run.
func writeStepSummary(pl plan.Plan, results []StepResult, cov *CoverageResult, w io.Writer) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(w, "star-ci: warning: could not open step summary: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(RenderSummary(pl, results, cov)); err != nil {
		fmt.Fprintf(w, "star-ci: warning: could not write step summary: %v\n", err)
	}
}
