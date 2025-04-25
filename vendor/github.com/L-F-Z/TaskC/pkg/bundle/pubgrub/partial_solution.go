// Copyright 2025 Fengzhi Li
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The following code includes modifications and references to code from other open source projects.
// https://github.com/mircearoata/pubgrub-go/

package pubgrub

import (
	"fmt"
	"slices"

	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type assignment interface {
	Package() string
	DecisionLevel() int
}

type partialSolution struct {
	assignments []assignment
}

type derivation struct {
	t             Term
	cause         *Incompatibility
	decisionLevel int
}

func (d derivation) String() string {
	var pos string
	if d.t.positive {
		pos = "+ Positive Term"
	} else {
		pos = "- Negative Term"
	}
	return fmt.Sprintf("----Derivation\n%s\nTerm.pkg : %s\nTerm.ver : %+v\nCause    : %+v\nLevel    : %d\n",
		pos,
		d.t.pkg,
		d.t.versionConstraint,
		d.cause,
		d.decisionLevel,
	)
}

func (d derivation) Package() string {
	return d.t.pkg
}

func (d derivation) DecisionLevel() int {
	return d.decisionLevel
}

type decision struct {
	pkg           string
	version       repointerface.Version
	blueprintID   string
	prefabID      string
	depends       []string
	dcontext      *dcontext.DeployContext
	decisionLevel int
}

func (d decision) String() string {
	return fmt.Sprintf("----Decision\npkg      : %s\nver      : %+v\nblueprint: %+v\nprefab   : %+v\nLevel    : %d\nContext  :%+v\n",
		d.pkg,
		d.version,
		d.blueprintID,
		d.prefabID,
		d.decisionLevel,
		d.dcontext,
	)
}

func (d decision) Package() string {
	return d.pkg
}

func (d decision) DecisionLevel() int {
	return d.decisionLevel
}

func (ps *partialSolution) get(pkg string) *Term {
	var result *Term
	for _, a := range ps.assignments {
		if a.Package() == pkg {
			if dec, ok := a.(decision); ok {
				return &Term{
					pkg:               dec.pkg,
					versionConstraint: repointerface.SingleVersionConstraint(dec.version),
					positive:          true,
				}
			}
			if der, ok := a.(derivation); ok {
				if result == nil {
					result = &der.t
				} else {
					intersection := result.intersect(der.t)
					result = &intersection
				}
			}
		}
	}
	return result
}

func (ps *partialSolution) currentDecisionLevel() int {
	currentDecisionLevel := 0
	for _, a := range ps.assignments {
		if _, ok := a.(decision); ok {
			currentDecisionLevel++
		}
	}
	return currentDecisionLevel
}

func (ps *partialSolution) add(t Term, cause *Incompatibility) {
	newDerivation := derivation{
		t:             t,
		decisionLevel: ps.currentDecisionLevel(),
		cause:         cause,
	}

	ps.assignments = append(ps.assignments, newDerivation)
}

func (ps *partialSolution) prefix(size int) partialSolution {
	return partialSolution{
		assignments: slices.Clone(ps.assignments[:size]),
	}
}

func (ps *partialSolution) findPositiveUndecided() string {
	decidedPackages := make(map[string]bool)
	for _, a := range ps.assignments {
		if _, ok := a.(decision); ok {
			decidedPackages[a.Package()] = true
		}
	}
	for _, a := range ps.assignments {
		if der, ok := a.(derivation); ok {
			if _, ok := decidedPackages[der.t.pkg]; der.t.positive && !ok {
				return der.t.pkg
			}
		}
	}
	return ""
}

func (ps *partialSolution) allPositiveUndecided() []string {
	decidedPackages := make(map[string]bool)
	for _, a := range ps.assignments {
		if _, ok := a.(decision); ok {
			decidedPackages[a.Package()] = true
		}
	}
	var undecidedPackages []string
	for _, a := range ps.assignments {
		if der, ok := a.(derivation); ok {
			if _, ok := decidedPackages[der.t.pkg]; der.t.positive && !ok {
				undecidedPackages = append(undecidedPackages, der.t.pkg)
			}
		}
	}
	return undecidedPackages
}

type SolvedItem struct {
	PrefabID    string
	BlueprintID string
	Depends     []string
}

func (ps *partialSolution) decisionsMap() map[string]SolvedItem {
	result := map[string]SolvedItem{}
	for _, a := range ps.assignments {
		if dec, ok := a.(decision); ok {
			result[dec.pkg] = SolvedItem{
				PrefabID:    dec.prefabID,
				BlueprintID: dec.blueprintID,
				Depends:     dec.depends,
			}
		}
	}
	return result
}

func (ps *partialSolution) collectContext() *dcontext.DeployContext {
	var ctx dcontext.DeployContext
	for _, a := range ps.assignments {
		if dec, ok := a.(decision); ok {
			ctx.Merge(dec.dcontext)
		}
	}
	return &ctx
}
