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

import "fmt"

type Incompatibility struct {
	terms     map[string]Term
	causes    []*Incompatibility
	dependant string
}

func (in Incompatibility) String() string {
	s := "\n~~~~~~~~~~~~~~~~~~~~~~~~~~\n"
	s += "DEPENDANT: " + in.dependant + "\n"
	for key, term := range in.terms {
		s += fmt.Sprintf("TERM [%s]\n%+v\n", key, term)
	}
	s += fmt.Sprintf("CAUSES: len = %d\n", len(in.causes))
	s += "~~~~~~~~~~~~~~~~~~~~~~~~~~\n"
	return s
}

func (in Incompatibility) Terms() []Term {
	terms := make([]Term, 0, len(in.terms))
	for _, t := range in.terms {
		terms = append(terms, t)
	}
	return terms
}

func (in Incompatibility) Causes() []*Incompatibility {
	return in.causes
}

func (in Incompatibility) get(pkg string) *Term {
	if t, ok := in.terms[pkg]; ok {
		return &t
	}
	return nil
}

type setRelation int

const (
	setRelationSatisfied setRelation = iota
	setRelationContradicted
	setRelationAlmostSatisfied
	setRelationInconclusive
)

func (in Incompatibility) relation(ps *partialSolution) (setRelation, *Term) {
	result := setRelationSatisfied
	var unsatisfied Term

	// The iteration order does not matter here,
	// since for an almost satisfied relation there is a single inconclusive term
	for _, t := range in.terms {
		t2 := ps.get(t.pkg)
		if t2 != nil {
			rel := t.relation(*t2)
			if rel == termRelationSatisfied {
				continue
			}
			if rel == termRelationContradicted {
				result = setRelationContradicted
				unsatisfied = t
				break
			}
		}

		// Either term inconclusive, or not present
		if result == setRelationSatisfied {
			result = setRelationAlmostSatisfied
			unsatisfied = t
		} else {
			result = setRelationInconclusive
		}
	}

	if result == setRelationSatisfied || result == setRelationInconclusive {
		return result, nil
	}
	return result, &unsatisfied
}

func (in Incompatibility) makePriorCause(c *Incompatibility, satisfier string) *Incompatibility {
	newIncompatibility := &Incompatibility{
		terms:  make(map[string]Term),
		causes: []*Incompatibility{&in, c},
	}
	for _, t := range in.terms {
		if t.pkg != satisfier {
			newIncompatibility.add(t)
		}
	}
	for _, t := range c.Terms() {
		if t.pkg != satisfier {
			newIncompatibility.add(t)
		}
	}
	return newIncompatibility
}

func (in Incompatibility) add(t Term) {
	existingTerm := in.get(t.pkg)
	if existingTerm != nil {
		in.terms[t.pkg] = existingTerm.intersect(t)
	} else {
		in.terms[t.pkg] = t
	}
}
