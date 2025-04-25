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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

// CRITICAL TODO: Wrong Decision Order!!!!
// The order affects deployment context

type solver struct {
	ps                *prefabservice.PrefabService
	rootPkg           string
	incompatibilities []*Incompatibility
	partialSolution   partialSolution
	dcontext          *dcontext.DeployContext
}

func (s *solver) Log() {
	fmt.Println("#######InCompatibilities##########")
	fmt.Println(len(s.incompatibilities))
	for _, incom := range s.incompatibilities {
		fmt.Printf("%+v\n\n", *incom)
	}
	fmt.Println("--------Partial Solution----------")
	fmt.Println(len(s.partialSolution.assignments))
	for _, assign := range s.partialSolution.assignments {
		fmt.Printf("%+v", assign)
	}
	fmt.Print("#################################\n\n")
}

func Solve(ps *prefabservice.PrefabService, repoType string, name string, version string, deps [][]*prefab.Prefab, ctx *dcontext.DeployContext) (map[string]SolvedItem, *dcontext.DeployContext, error) {
	if len(deps) == 0 {
		return nil, ctx, nil
	}

	// ####### ADD ROOT INFO #######
	rootKey := GenKey(repoType, name)
	rootVer, err := prefabservice.ParseAnyVersion(repoType, version)
	if err != nil {
		return nil, nil, err
	}
	rootTerm := Term{
		pkg:               rootKey,
		versionConstraint: repointerface.SingleVersionConstraint(rootVer),
		positive:          false,
	}
	rootIncompatibility := &Incompatibility{terms: map[string]Term{rootKey: rootTerm}}
	s := solver{
		ps:                ps,
		rootPkg:           rootKey,
		incompatibilities: []*Incompatibility{rootIncompatibility},
		dcontext:          ctx,
	}
	s.partialSolution.add(rootTerm.Negate(), rootIncompatibility)

	dependencies, err := selectDependency(deps, s.dcontext)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select bundle dependencies: [%v]", err)
	}

	for _, depItem := range dependencies {
		s.addIncompatibility(&Incompatibility{
			terms: map[string]Term{
				rootKey: {
					pkg:               rootKey,
					versionConstraint: repointerface.SingleVersionConstraint(rootVer),
					positive:          true,
				},
				depItem.name: {
					pkg:               depItem.name,
					versionConstraint: depItem.specifier,
					positive:          false,
				},
			},
			dependant: rootKey,
		})
	}
	s.partialSolution.assignments = append(s.partialSolution.assignments, decision{
		pkg:           rootKey,
		version:       rootVer,
		dcontext:      ctx,
		decisionLevel: s.partialSolution.currentDecisionLevel() + 1,
	})
	s.dcontext = s.partialSolution.collectContext()

	// ####### START SOLVING #######
	next := rootKey
	for {
		err := s.unitPropagation(next)
		if err != nil {
			return nil, nil, err
		}

		// Prefetch all positive undecided packages
		// undecided := s.partialSolution.allPositiveUndecided()
		// go func() {
		// 	for _, pkg := range undecided {
		// 		go func(pkg string) {
		// 			_, _ = s.source.GetPackageVersions(pkg)
		// 		}(pkg)
		// 	}
		// }()

		var done bool
		next, done, err = s.decision()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to make decision: [%v]", err)
		}
		if done {
			break
		}
	}
	result := s.partialSolution.decisionsMap()
	delete(result, rootKey)
	return result, s.dcontext, nil
}

func (s *solver) unitPropagation(inPkg string) error {
	changed := []string{inPkg}
	var contradictedIncompatibilities []*Incompatibility
	for len(changed) > 0 {
		pkg := changed[0]
		changed = changed[1:]

		for i := len(s.incompatibilities) - 1; i >= 0; i-- {
			currentIncompatibility := s.incompatibilities[i]
			if slices.Contains(contradictedIncompatibilities, currentIncompatibility) {
				continue
			}
			hasPkg := false
			for _, t := range currentIncompatibility.terms {
				if t.pkg == pkg {
					hasPkg = true
					break
				}
			}
			if !hasPkg {
				continue
			}

			rel, t := currentIncompatibility.relation(&s.partialSolution)
			if rel == setRelationSatisfied {
				newIncompatibility, err := s.conflictResolution(currentIncompatibility)
				if err != nil {
					return err
				}
				newRel, newT := newIncompatibility.relation(&s.partialSolution)
				if newRel != setRelationAlmostSatisfied {
					return errors.New("new incompatibility is not almost satisfied, this should never happen")
				}
				s.partialSolution.add(newT.Negate(), newIncompatibility)
				changed = []string{newT.pkg}
				contradictedIncompatibilities = append(contradictedIncompatibilities, newIncompatibility)
				break
			} else if rel == setRelationAlmostSatisfied {
				s.partialSolution.add(t.Negate(), currentIncompatibility)
				changed = append(changed, t.pkg)
			}
			contradictedIncompatibilities = append(contradictedIncompatibilities, currentIncompatibility)
		}
	}
	return nil
}

func (s *solver) conflictResolution(fromIncompatibility *Incompatibility) (*Incompatibility, error) {
	incompatibilityChanged := false
	for {
		if s.isIncompatibilityTerminal(fromIncompatibility) {
			return nil, SolvingError{fromIncompatibility}
		}

		satisfierIdx := BinarySearchFunc(0, len(s.partialSolution.assignments), func(i int) bool {
			prefix := s.partialSolution.prefix(i + 1)
			rel, _ := fromIncompatibility.relation(&prefix)
			return rel == setRelationSatisfied
		})
		satisfier := s.partialSolution.assignments[satisfierIdx]

		incompatibilityTerm := fromIncompatibility.get(satisfier.Package())

		previousSatisfierIdx := BinarySearchFunc(-1, satisfierIdx+1, func(i int) bool {
			prefix := s.partialSolution.prefix(i + 1)
			prefix.assignments = append(prefix.assignments, satisfier)
			rel, _ := fromIncompatibility.relation(&prefix)
			return rel == setRelationSatisfied
		})
		var previousSatisfier assignment
		previousSatisfierLevel := 1
		if previousSatisfierIdx >= 0 {
			previousSatisfier = s.partialSolution.assignments[previousSatisfierIdx]
			previousSatisfierLevel = previousSatisfier.DecisionLevel()
		}

		if _, ok := satisfier.(decision); ok || previousSatisfierLevel != satisfier.DecisionLevel() {
			if incompatibilityChanged {
				s.addIncompatibility(fromIncompatibility)
			}

			decLevel := 0
			for i := range len(s.partialSolution.assignments) {
				if _, ok := s.partialSolution.assignments[i].(decision); ok {
					decLevel++
					if decLevel > previousSatisfierLevel {
						s.partialSolution = s.partialSolution.prefix(i)
						break
					}
				}
			}

			return fromIncompatibility, nil
		}

		der := satisfier.(derivation)

		priorCause := fromIncompatibility.makePriorCause(der.cause, satisfier.Package())

		if rel := incompatibilityTerm.relation(der.t); rel != termRelationSatisfied {
			priorCause.add(der.t.difference(*incompatibilityTerm).Negate())
		}

		fromIncompatibility = priorCause
		incompatibilityChanged = true
	}
}

func (s *solver) decision() (string, bool, error) {
	pkg := s.partialSolution.findPositiveUndecided()
	if pkg == "" {
		return "", true, nil
	}

	t := s.partialSolution.get(pkg)
	repoType, name, err := GetTypeName(t.pkg)
	if err != nil {
		return pkg, false, fmt.Errorf("failed to decode package name [%v]", t.pkg)
	}

	// fmt.Printf("@ Requesting Blueprint for %s %s, %+v\n", repoType, name, t.versionConstraint)
	blueprint, blueprintID, prefabID, err := s.ps.RequestBlueprint(repoType, name, t.versionConstraint, s.dcontext)
	if err != nil {
		return pkg, false, fmt.Errorf("failed to get package %s blueprint: [%v]", t.pkg, err)
	}
	if blueprint == nil {
		fmt.Printf("BLUEPRINT IS NIL")
		s.addIncompatibility(&Incompatibility{
			terms: map[string]Term{pkg: *t},
		})
		return pkg, false, nil
	}
	chosenVersion, err := prefabservice.ParseAnyVersion(blueprint.Type, blueprint.Version)
	if err != nil {
		return pkg, false, fmt.Errorf("failed to parse version %s: [%v]", blueprint.Version, err)
	}

	dependencies, err := selectDependency(blueprint.Depend, s.dcontext)
	if err != nil {
		return pkg, false, fmt.Errorf("failed to select dependencies: [%v]", err)
	}

	var depends []string
	for _, depItem := range dependencies {
		s.addIncompatibility(&Incompatibility{
			terms: map[string]Term{
				pkg: {
					pkg:               pkg,
					versionConstraint: repointerface.SingleVersionConstraint(chosenVersion),
					positive:          true,
				},
				depItem.name: {
					pkg:               depItem.name,
					versionConstraint: depItem.specifier,
					positive:          false,
				},
			},
			dependant: pkg,
		})
		depends = append(depends, depItem.name)
	}

	s.partialSolution.assignments = append(s.partialSolution.assignments, decision{
		pkg:           t.pkg,
		version:       chosenVersion,
		blueprintID:   blueprintID,
		prefabID:      prefabID,
		depends:       depends,
		dcontext:      blueprint.Context,
		decisionLevel: s.partialSolution.currentDecisionLevel() + 1,
	})
	s.dcontext = s.partialSolution.collectContext()

	return pkg, false, nil
}

func (s *solver) addIncompatibility(in *Incompatibility) {
	if slices.ContainsFunc(s.incompatibilities, func(i *Incompatibility) bool {
		return maps.EqualFunc(i.terms, in.terms, func(a, b Term) bool {
			return a.Equal(b)
		})
	}) {
		return
	}
	s.incompatibilities = append(s.incompatibilities, in)
}

func (s *solver) isIncompatibilityTerminal(in *Incompatibility) bool {
	if len(in.terms) == 0 {
		return true
	}
	if len(in.terms) == 1 {
		for _, t := range in.terms {
			if t.positive && t.pkg == s.rootPkg {
				return true
			}
		}
	}
	return false
}

// BinarySearchFunc returns the smallest index i in [low, high) at which f(i) is true,
// assuming that on the range [low, high), f(i) == true implies f(i+1) == true.
func BinarySearchFunc(low, high int, f func(int) bool) int {
	// For this function to work with negative indices as well,
	// the power of 2 step based variation of binary search is used
	i := low - 1
	step := 1
	for step < (high - low) {
		step <<= 1
	}
	step >>= 1

	// We find the last index for which f(i) is false
	for ; step > 0; step >>= 1 {
		if i+step < high && !f(i+step) {
			i += step
		}
	}

	// Then f(i+1) will be true
	return i + 1
}

type depItem struct {
	name      string
	specifier repointerface.Constraint
}

// add blueprint's context to current deployment context
// return ctx.Merge(blueprint.Context)
func selectDependency(alternatives [][]*prefab.Prefab, ctx *dcontext.DeployContext) (dependencies []depItem, err error) {
	for _, alternative := range alternatives {
		best := 0
		var selected *prefab.Prefab
		for _, cand := range alternative {
			var deployability int
			deployability, err = ctx.Evaluate(cand.Deployability)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate deployability for %s: [%v]", cand.Name, err)
			}
			if deployability > best {
				best = deployability
				selected = cand
			}
		}
		if best == 0 || selected == nil {
			// when only one alternative, and it has a deployability requirement
			// then no prefab is needed when the deployability is 0
			if len(alternative) == 1 && alternative[0] != nil {
				if alternative[0].Deployability != nil && len(*alternative[0].Deployability) != 0 {
					continue
				}
			}
			return nil, fmt.Errorf("no alternative prefab is deployable: %s", alternative)
		}
		specifier, err := prefabservice.DecodeAnySpecifier(selected.SpecType, selected.Specifier)
		if err != nil {
			return nil, fmt.Errorf("failed to decode specifier %s: [%v]", selected.Specifier, err)
		}
		dependencies = append(dependencies, depItem{
			name:      GenKey(selected.SpecType, selected.Name),
			specifier: specifier,
		})
	}
	slices.Reverse(dependencies)
	return
}

func GenKey(repoType string, name string) string {
	if repoType == "PyPI" {
		name = normalizeName(name)
	}
	return repoType + " " + name
}

func GetTypeName(key string) (repoType string, name string, err error) {
	parts := strings.SplitN(key, " ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid key format")
	}
	return parts[0], parts[1], nil
}

func normalizeName(name string) string {
	replaced := strings.ReplaceAll(name, "-", "-")
	replaced = strings.ReplaceAll(replaced, "_", "-")
	replaced = strings.ReplaceAll(replaced, ".", "-")
	replaced = strings.ToLower(replaced)
	return replaced
}
