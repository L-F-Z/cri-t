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

package repointerface

import (
	"encoding/json"
	"slices"
)

type Version interface {
	Compare(Version) int
	String() string
}

// VersionRange represents a continuous range of versions
type VersionRange struct {
	LowerBound     Version
	UpperBound     Version
	LowerInclusive bool
	UpperInclusive bool
}

type Constraint struct {
	RepoType string
	Ranges   []VersionRange
	Raw      string
}

func (c Constraint) String() string {
	return c.Raw
}

type VersionRangeString struct {
	LowerBound     string `json:"lower_bound"`
	UpperBound     string `json:"upper_bound"`
	LowerInclusive bool   `json:"lower_inclusive"`
	UpperInclusive bool   `json:"upper_inclusive"`
}
type ConstraintString struct {
	RepoType string               `json:"repo_type"`
	Ranges   []VersionRangeString `json:"ranges"`
	Raw      string               `json:"raw"`
}

func (c Constraint) Encode() (string, error) {
	enc := ConstraintString{
		RepoType: c.RepoType,
		Ranges:   make([]VersionRangeString, len(c.Ranges)),
		Raw:      c.Raw,
	}
	for i, ver := range c.Ranges {
		lower, upper := "", ""
		if ver.LowerBound != nil {
			lower = ver.LowerBound.String()
		}
		if ver.UpperBound != nil {
			upper = ver.UpperBound.String()
		}
		enc.Ranges[i] = VersionRangeString{
			LowerBound:     lower,
			UpperBound:     upper,
			LowerInclusive: ver.LowerInclusive,
			UpperInclusive: ver.UpperInclusive,
		}
	}
	bytes, err := json.Marshal(enc)
	return string(bytes), err
}

var AnyConstraint = Constraint{
	Ranges: []VersionRange{{
		LowerBound:     nil,
		UpperBound:     nil,
		LowerInclusive: false,
		UpperInclusive: false,
	}},
	Raw: "any",
}

func (c Constraint) FilterAndSort(versions []Version) []Version {
	versions = slices.DeleteFunc(versions, func(v Version) bool { return !c.Contains(v) })
	slices.SortFunc(versions, func(a Version, b Version) int { return -a.Compare(b) })
	return versions
}

func (c Constraint) IsAny() bool {
	return len(c.Ranges) == 1 && c.Ranges[0].equal(VersionRange{})
}

func (c Constraint) IsEmpty() bool {
	return len(c.Ranges) == 0
}

func (c Constraint) Equal(other Constraint) bool {
	return slices.EqualFunc(c.Ranges, other.Ranges, func(a, b VersionRange) bool { return a.equal(b) })
}

func (c *Constraint) AddRange(lowerBound Version, upperBound Version, lowerInclusive bool, upperInclusive bool) {
	c.Ranges = append(c.Ranges, VersionRange{lowerBound, upperBound, lowerInclusive, upperInclusive})
}

func (c Constraint) Contains(other Version) bool {
	for _, r := range c.Ranges {
		if r.contains(other) {
			return true
		}
	}
	return false
}

func (c Constraint) Intersect(other Constraint) Constraint {
	if c.IsEmpty() || other.IsEmpty() {
		return Constraint{}
	}
	new := Constraint{Raw: c.Raw, RepoType: c.RepoType}
	for _, r := range c.Ranges {
		for _, r2 := range other.Ranges {
			intersection := r.intersect(r2)
			if !intersection.isEmpty() {
				new.Ranges = append(new.Ranges, intersection)
			}
		}
	}
	return new.canonical()
}

func (c Constraint) Union(other Constraint) Constraint {
	c.Ranges = append(c.Ranges, other.Ranges...)
	return c.canonical()
}

func (c Constraint) Inverse() Constraint {
	result := AnyConstraint
	result.RepoType = c.RepoType
	for _, r := range c.Ranges {
		result = result.Intersect(r.inverse())
	}
	return result.canonical()
}

func (c Constraint) Difference(other Constraint) Constraint {
	return c.Intersect(other.Inverse())
}

// canonical returns a new Constraint that is equivalent to v
// but which contains no two overlapping ranges, and which
// is sorted in ascending order of the lower bound of each range.
func (c Constraint) canonical() Constraint {
	type versionOnAxis struct {
		version     Version
		isInclusive bool
		isUpper     bool
	}

	versions := make([]versionOnAxis, 0, 2*len(c.Ranges))
	for _, r := range c.Ranges {
		versions = append(versions, versionOnAxis{
			version:     r.LowerBound,
			isInclusive: r.LowerInclusive,
			isUpper:     false,
		})
		versions = append(versions, versionOnAxis{
			version:     r.UpperBound,
			isInclusive: r.UpperInclusive,
			isUpper:     true,
		})
	}

	slices.SortFunc(versions, func(a, b versionOnAxis) int {
		if a.version != nil && b.version != nil {
			result := a.version.Compare(b.version)
			if result != 0 {
				return result
			}
			// If the versions are equal, order the lower bound before the upper bound for the merge to continue,
			// but only if the one of the bounds is inclusive
			if a.isUpper != b.isUpper && (a.isInclusive || b.isInclusive) {
				if a.isUpper {
					return 1
				}
				return -1
			}
			// If the versions are the same version and type, order the inclusive bound at the outer point based on type
			if a.isInclusive != b.isInclusive {
				if a.isUpper {
					if a.isInclusive {
						return 1
					}
					return -1
				}
				if a.isInclusive {
					return -1
				}
				return 1
			}

			// everything is equal
			return 0
		}
		if a.version == nil && b.version == nil {
			if a.isUpper != b.isUpper {
				if a.isUpper {
					return 1
				}
				return -1
			}
			return 0
		}
		if a.version == nil {
			if a.isUpper {
				return 1
			}
			return -1
		}
		if b.version == nil {
			if b.isUpper {
				return -1
			}
			return 1
		}
		return 0
	})

	result := Constraint{Raw: c.Raw, RepoType: c.RepoType}

	nestedCount := 0
	var currentRange VersionRange
	for i := range len(versions) {
		if versions[i].isUpper {
			nestedCount--
		} else {
			nestedCount++
			if nestedCount == 1 {
				currentRange = VersionRange{
					LowerBound:     versions[i].version,
					LowerInclusive: versions[i].isInclusive,
				}
			}
		}
		if nestedCount == 0 {
			currentRange.UpperBound = versions[i].version
			currentRange.UpperInclusive = versions[i].isInclusive
			result.Ranges = append(result.Ranges, currentRange)
		}
	}

	// At this point no two ranges are overlapping, therefore no two ranges have an equal lower bound
	slices.SortFunc(result.Ranges, func(a, b VersionRange) int {
		if a.LowerBound == nil && b.LowerBound == nil {
			return 0
		}
		if a.LowerBound == nil {
			return -1
		}
		if b.LowerBound == nil {
			return 1
		}
		return a.LowerBound.Compare(b.LowerBound)
	})

	return result
}

// Ranges-----------------

func (r VersionRange) contains(other Version) bool {
	if r.LowerBound != nil {
		result := r.LowerBound.Compare(other)
		if r.LowerInclusive {
			if result > 0 { // lower bound is greater than other
				return false
			}
		} else {
			if result >= 0 { // lower bound is greater than or equal to other
				return false
			}
		}
	}
	if r.UpperBound != nil {
		result := r.UpperBound.Compare(other)
		if r.UpperInclusive {
			if result < 0 { // upper bound is less than other
				return false
			}
		} else {
			if result <= 0 { // upper bound is less than or equal to other
				return false
			}
		}
	}
	return true
}

func (r VersionRange) isEmpty() bool {
	if r.LowerBound != nil && r.UpperBound != nil {
		result := r.LowerBound.Compare(r.UpperBound)
		if result > 0 {
			// lower bound is greater than upper bound
			return true
		} else if result == 0 && (!r.LowerInclusive || !r.UpperInclusive) {
			// lower bound is equal to upper bound, but one of them is not inclusive
			return true
		}
	}
	return false
}

// TODO: Set RepoType
func (r VersionRange) inverse() Constraint {
	if r.UpperBound == nil && r.LowerBound == nil {
		return Constraint{}
	}
	var new Constraint
	if r.LowerBound != nil {
		new.Ranges = append(new.Ranges, VersionRange{
			UpperBound:     r.LowerBound,
			UpperInclusive: !r.LowerInclusive,
		})
	}
	if r.UpperBound != nil {
		new.Ranges = append(new.Ranges, VersionRange{
			LowerBound:     r.UpperBound,
			LowerInclusive: !r.UpperInclusive,
		})
	}
	return new.canonical()
}

func (r VersionRange) intersect(other VersionRange) (result VersionRange) {
	if r.LowerBound == nil {
		result.LowerBound = other.LowerBound
		result.LowerInclusive = other.LowerInclusive
	} else {
		if other.LowerBound == nil {
			result.LowerBound = r.LowerBound
			result.LowerInclusive = r.LowerInclusive
		} else {
			cmp := r.LowerBound.Compare(other.LowerBound)
			switch {
			case cmp < 0:
				result.LowerBound = other.LowerBound
				result.LowerInclusive = other.LowerInclusive
			case cmp > 0:
				result.LowerBound = r.LowerBound
				result.LowerInclusive = r.LowerInclusive
			default:
				result.LowerBound = r.LowerBound
				result.LowerInclusive = r.LowerInclusive && other.LowerInclusive
			}
		}
	}
	if r.UpperBound == nil {
		result.UpperBound = other.UpperBound
		result.UpperInclusive = other.UpperInclusive
	} else {
		if other.UpperBound == nil {
			result.UpperBound = r.UpperBound
			result.UpperInclusive = r.UpperInclusive
		} else {
			cmp := r.UpperBound.Compare(other.UpperBound)
			switch {
			case cmp < 0:
				result.UpperBound = r.UpperBound
				result.UpperInclusive = r.UpperInclusive
			case cmp > 0:
				result.UpperBound = other.UpperBound
				result.UpperInclusive = other.UpperInclusive
			default:
				result.UpperBound = r.UpperBound
				result.UpperInclusive = r.UpperInclusive && other.UpperInclusive
			}
		}
	}
	return
}

func (r VersionRange) equal(other VersionRange) bool {
	if r.LowerBound != nil && other.LowerBound != nil {
		if r.LowerBound.Compare(other.LowerBound) != 0 {
			return false
		}
		if r.LowerInclusive != other.LowerInclusive {
			return false
		}
	} else if r.LowerBound != nil || other.LowerBound != nil {
		return false
	}
	if r.UpperBound != nil && other.UpperBound != nil {
		if r.UpperBound.Compare(other.UpperBound) != 0 {
			return false
		}
		if r.UpperInclusive != other.UpperInclusive {
			return false
		}
	} else if r.UpperBound != nil || other.UpperBound != nil {
		return false
	}
	return true
}

// NewConstraintFromVersionSubset returns a minimal constraint that matches exactly the given versions out of the
// given set of all versions. Both slices must be sorted in ascending order
func NewConstraintFromVersionSubset(versions []Version, allVersions []Version) (c Constraint) {
	i := 0
	for _, v := range versions {
		for ; i < len(allVersions); i++ {
			if allVersions[i].Compare(v) == 0 {
				break
			}
		}
		if i == len(allVersions) {
			panic("should be unreachable")
		}

		if i == 0 {
			c.AddRange(nil, allVersions[i], false, true)
		}
		if i < len(allVersions)-1 {
			c.AddRange(allVersions[i], allVersions[i+1], true, true)
		} else {
			c.AddRange(allVersions[i], nil, true, false)
		}
	}
	// TODO! generate c.Raw
	c.Raw = ""
	return c.canonical()
}

func SingleVersionConstraint(v Version) (c Constraint) {
	c.AddRange(v, v, true, true)
	c.Raw = v.String()
	return
}
