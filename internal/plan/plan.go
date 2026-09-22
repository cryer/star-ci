// Package plan defines CI steps and the executable plan produced by rules.
package plan

import "github.com/star-ci/star-ci/internal/profile"

// Category groups steps; categories also define execution order.
type Category string

const (
	CatInstall   Category = "install"
	CatLint      Category = "lint"
	CatTypecheck Category = "typecheck"
	CatTest      Category = "test"
	CatBuild     Category = "build"
	CatSecurity  Category = "security"
)

// CategoryOrder is the fixed execution order of categories.
var CategoryOrder = []Category{
	CatInstall,
	CatLint,
	CatTypecheck,
	CatTest,
	CatBuild,
	CatSecurity,
}

// Step is one CI step: a named group of shell commands run from the repo root.
type Step struct {
	ID       string   // unique stable id, e.g. "node-install", "go-test"
	Name     string   // human-readable display name
	Category Category // grouping + ordering
	Commands []string // shell commands executed sequentially in the repo root
	Reason   string   // explainability: which signals caused this step
	Optional bool     // optional steps warn instead of failing the run
}

// Plan is the full CI plan for a repository.
type Plan struct {
	Steps   []Step          // ordered by CategoryOrder, stable within a category
	Profile profile.Profile // the portrait this plan was derived from
}

// Sort orders steps by CategoryOrder, keeping insertion order within a category.
func (p *Plan) Sort() {
	rank := func(c Category) int {
		for i, o := range CategoryOrder {
			if o == c {
				return i
			}
		}
		return len(CategoryOrder)
	}
	// stable insertion sort keeps within-category order deterministic
	for i := 1; i < len(p.Steps); i++ {
		for j := i; j > 0 && rank(p.Steps[j-1].Category) > rank(p.Steps[j].Category); j-- {
			p.Steps[j-1], p.Steps[j] = p.Steps[j], p.Steps[j-1]
		}
	}
}

// RequiredSteps returns non-optional steps.
func (p *Plan) RequiredSteps() []Step {
	var out []Step
	for _, s := range p.Steps {
		if !s.Optional {
			out = append(out, s)
		}
	}
	return out
}
