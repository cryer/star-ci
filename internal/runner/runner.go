// Package runner executes a CI plan locally, streaming output.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"github.com/cryer/star-ci/internal/plan"
)

// Run prints and executes each step of the plan from root. A failed required
// step aborts the run with an error; a failed optional step only warns.
func Run(ctx context.Context, root string, pl plan.Plan, w io.Writer) error {
	printHeader(pl, w)
	passed, warnings := 0, 0
	results := make([]StepResult, 0, len(pl.Steps))
	for i, s := range pl.Steps {
		printStep(s, w)
		var capture bytes.Buffer
		failed := false
		for _, c := range s.Commands {
			if err := runCommand(ctx, root, c, io.MultiWriter(w, &capture)); err != nil {
				failed = true
				if s.Optional {
					warnings++
					fmt.Fprintf(w, "    warning: optional step %s failed: %v\n", s.ID, err)
					results = append(results, StepResult{Step: s, Status: StatusWarned, Output: capture.String()})
				} else {
					fmt.Fprintf(w, "    failure: step %s failed: %v\n", s.ID, err)
					results = append(results, StepResult{Step: s, Status: StatusFailed, Output: capture.String()})
					for _, rest := range pl.Steps[i+1:] {
						results = append(results, StepResult{Step: rest, Status: StatusSkipped})
					}
					err = fmt.Errorf("step %s (%s) failed: %w", s.ID, s.Name, err)
					if d := Digest(capture.String(), DigestLines); d != "" {
						err = fmt.Errorf("%w\n\nfailure digest:\n%s", err, d)
					}
					writeStepSummary(pl, results, w)
					return err
				}
				break
			}
		}
		if !failed {
			passed++
			results = append(results, StepResult{Step: s, Status: StatusPassed, Output: capture.String()})
		}
	}
	fmt.Fprintf(w, "star-ci: %d steps passed, %d optional warnings\n", passed, warnings)
	writeStepSummary(pl, results, w)
	return nil
}

// Explain prints the plan without executing any command.
func Explain(pl plan.Plan, w io.Writer) {
	printHeader(pl, w)
	for _, s := range pl.Steps {
		printStep(s, w)
	}
	fmt.Fprintf(w, "star-ci: %d steps (dry run, nothing executed)\n", len(pl.Steps))
}

func printHeader(pl plan.Plan, w io.Writer) {
	var langs []string
	for _, l := range pl.Profile.Languages {
		if l.VersionHint != "" {
			langs = append(langs, l.Name+" "+l.VersionHint)
		} else {
			langs = append(langs, l.Name)
		}
	}
	if len(langs) == 0 {
		fmt.Fprintf(w, "star-ci: no languages detected, %d steps\n", len(pl.Steps))
	} else {
		fmt.Fprintf(w, "star-ci: detected %s, %d steps\n", strings.Join(langs, ", "), len(pl.Steps))
	}
}

func printStep(s plan.Step, w io.Writer) {
	fmt.Fprintf(w, "==> %s (%s)\n", s.Name, s.Category)
	fmt.Fprintf(w, "    reason: %s\n", s.Reason)
	for _, c := range s.Commands {
		fmt.Fprintf(w, "    $ %s\n", c)
	}
}

func runCommand(ctx context.Context, dir, command string, w io.Writer) error {
	shell, args := shellArgs(command)
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// shellArgs prefers sh; on Windows without sh in PATH it falls back to cmd.
func shellArgs(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("sh"); err != nil {
			return "cmd", []string{"/C", command}
		}
	}
	return "sh", []string{"-c", command}
}
