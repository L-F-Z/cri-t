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

package apt

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type Version struct {
	Epoch           int
	UpstreamVersion string
	DebianRevision  string
}

func (v Version) String() string {
	s := ""
	if v.Epoch != 0 {
		s += fmt.Sprintf("%d:", v.Epoch)
	}
	s += v.UpstreamVersion
	if v.DebianRevision != "0" && v.DebianRevision != "" {
		s += "-" + v.DebianRevision
	}
	return s
}

var NullVersion = Version{Epoch: -1}

// parse debian package version string, the format is [epoch:]upstream_version[-debian_revision]
func ParseVersion(version string) (ver Version, err error) {
	// If there is no debian_revision then hyphens are not allowed
	// Break the version number apart at the last hyphen in the string (if there is one)
	// to determine the upstream_version and debian_revision.
	// The absence of a debian_revision is equivalent to a debian_revision of 0.
	lastHyphenIndex := strings.LastIndex(version, "-")
	if lastHyphenIndex != -1 {
		ver.UpstreamVersion = version[:lastHyphenIndex]
		ver.DebianRevision = version[lastHyphenIndex+1:]
	} else {
		ver.UpstreamVersion = version
		ver.DebianRevision = "0"
	}

	firstColonIndex := strings.Index(ver.UpstreamVersion, ":")
	if firstColonIndex == -1 {
		ver.Epoch = 0
		return
	}
	ver.Epoch, err = strconv.Atoi(ver.UpstreamVersion[:firstColonIndex])
	ver.UpstreamVersion = ver.UpstreamVersion[firstColonIndex+1:]
	return
}

// versions are compared according to https://www.debian.org/doc/debian-policy/ch-controlfields.html#version

// When comparing two version numbers, first the epoch of each are compared, then the upstream_version if epoch is equal,
// and then debian_revision if upstream_version is also equal. epoch is compared numerically.
// The upstream_version and debian_revision parts are compared by the package management system using the following algorithm:
//
// The strings are compared from left to right.
//
// First the initial part of each string consisting entirely of non-digit characters is determined.
// These two parts (one of which may be empty) are compared lexically. If a difference is found it is returned.
// The lexical comparison is a comparison of ASCII values modified so that all the letters sort earlier than all the non-letters
// and so that a tilde sorts before anything, even the end of a part.
// For example, the following parts are in sorted order from earliest to latest: ~~, ~~a, ~, the empty part, a.
//
// Then the initial part of the remainder of each string which consists entirely of digit characters is determined.
// The numerical values of these two parts are compared, and any difference found is returned as the result of the comparison.
// For these purposes an empty string (which can only occur at the end of one or both version strings being compared) counts as zero.
//
// These two steps (comparing and removing initial non-digit strings and initial digit strings) are repeated until a difference
// is found or both strings are exhausted.

func (a Version) Compare(other repointerface.Version) (result int) {
	b, ok := other.(Version)
	if !ok {
		log.Panicf("comparing with wrong type %+v, %T\n", other, other)
	}
	if a.Epoch != b.Epoch {
		return a.Epoch - b.Epoch
	}
	result = compareVersionString(a.UpstreamVersion, b.UpstreamVersion)
	if result != 0 {
		return
	}
	return compareVersionString(a.DebianRevision, b.DebianRevision)
}

// compareVersionString compares two parts according to the described rules.
func compareVersionString(a, b string) int {
	pa, pb := 0, 0
	var result, da, db int
	for pa != len(a) || pb != len(b) {
		da, db, result = compareLexical(a[pa:], b[pb:])
		// fmt.Println("Lexical", result, da, db)
		if result != 0 {
			return result
		}
		pa += da
		pb += db

		da, db, result = compareNumerical(a[pa:], b[pb:])
		// fmt.Println("Digit", result, da, db)
		if result != 0 {
			return result
		}
		pa += da
		pb += db
	}
	return 0
}

func compareNumerical(a, b string) (da, db, result int) {
	da, db = 0, 0
	numa, numb := 0, 0

	for da < len(a) && isDigit(a[da]) {
		da++
	}
	if da > 0 {
		numa, _ = strconv.Atoi(a[:da])
	}

	for db < len(b) && isDigit(b[db]) {
		db++
	}
	if db > 0 {
		numb, _ = strconv.Atoi(b[:db])
	}

	result = numa - numb
	return
}

// lexicalCompare compares two strings according to the special rules.
// returns 0 when a == b
// returns >0 when a is later
// returns <0 when b is later
func compareLexical(a, b string) (da, db, result int) {
	da, db = 0, 0
	for da < len(a) && !isDigit(a[da]) {
		da++
	}
	for db < len(b) && !isDigit(b[db]) {
		db++
	}

	minLen := utils.Min(da, db)
	for i := range minLen {
		if a[i] == b[i] {
			continue
		}
		if a[i] == '~' {
			result = -1
			return
		}
		if b[i] == '~' {
			result = 1
			return
		}
		if !isLetter(a[i]) && isLetter(b[i]) {
			result = 1
			return
		}
		if isLetter(a[i]) && !isLetter(b[i]) {
			result = -1
			return
		}
		result = int(a[i]) - int(b[i])
		return
	}
	if da > minLen && a[minLen] == '~' {
		result = -1
	} else if db > minLen && b[minLen] == '~' {
		result = 1
	} else {
		result = da - db
	}
	return
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
