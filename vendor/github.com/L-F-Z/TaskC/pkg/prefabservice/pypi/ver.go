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

package pypi

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type Version struct {
	Epoch   int      // Epoch number
	Release []int    // Release number
	Pre     string   // Pre-release type
	PreNum  int      // Pre-release number
	Post    string   // Post-release type
	PostNum int      // Post-release number
	Dev     string   // Dev-release type
	DevNum  int      // Dev-release number
	Local   []string // Local version label
	Raw     string
}

func (v Version) String() string {
	return v.Raw
}

func (v Version) stringer() string {
	var b strings.Builder
	if v.Epoch != 0 {
		fmt.Fprintf(&b, "%d!", v.Epoch)
	}
	if len(v.Release) > 0 {
		fmt.Fprintf(&b, "%d", v.Release[0])
		for _, r := range v.Release[1:] {
			fmt.Fprintf(&b, ".%d", r)
		}
	}
	if v.Pre != "" {
		fmt.Fprintf(&b, "%s%d", v.Pre, v.PreNum)
	}
	if v.Post != "" {
		fmt.Fprintf(&b, ".%s%d", v.Post, v.PostNum)
	}
	if v.Dev != "" {
		fmt.Fprintf(&b, ".%s%d", v.Dev, v.DevNum)
	}
	if len(v.Local) > 0 {
		b.WriteByte('+')
		b.WriteString(strings.Join(v.Local, "."))
	}
	return b.String()
}

var (
	ver_regexp *regexp.Regexp
)

func init() {
	ver_regexp = regexp.MustCompile(`^\s*v?` +
		`(?:` +
		`(?:([0-9]+)!)?` + // epoch
		`([0-9]+(?:\.[0-9]+)*)` + // release
		`([-_\.]?(a|b|c|rc|alpha|beta|pre|preview)[-_\.]?([0-9]+)?)?` + // pre-release
		`((?:-([0-9]+))|(?:[-_\.]?(post|rev|r)[-_\.]?([0-9]+)?))?` + // post-release
		`([-_\.]?(dev)[-_\.]?([0-9]+)?` + // dev release
		`)?)` +
		`(?:\+([a-z0-9]+(?:[-_\.][a-z0-9]+)*))?` + // local
		`\s*$`)
}

// since the valid pre/post/dev release strings are
// "a b c rc alpha beta pre preview post rev r dev",
// NegInfinity is smaller than each one of them,
// and Infinity is bigger than each one of them.
const NegInfinity = ""
const Infinity = "z"

func ParseVersion(version string) (ver Version, err error) {
	ver, err = parseVersion(version, nil)
	if err != nil {
		return
	}
	// Remove trailing zeros in ver.Release for comparing
	nonZero := len(ver.Release)
	for nonZero > 1 && ver.Release[nonZero-1] == 0 {
		nonZero--
	}
	ver.Release = ver.Release[:nonZero]
	return
}

func parseVersion(version string, modify func(*Version)) (ver Version, err error) {
	version = strings.ToLower(version)
	match := ver_regexp.FindStringSubmatch(version)
	if match == nil {
		err = errors.New("invalid version syntax")
		return
	}

	// Epoch segment
	if match[1] != "" {
		ver.Epoch, _ = strconv.Atoi(match[1])
	}

	// Release segment
	for release := range bytes.SplitSeq([]byte(match[2]), []byte(".")) {
		num, _ := strconv.Atoi(string(release))
		ver.Release = append(ver.Release, num)
	}

	// Pre-release segment
	if match[3] != "" {
		switch match[4] {
		case "a", "alpha":
			ver.Pre = "a"
		case "b", "beta":
			ver.Pre = "b"
		case "c", "rc", "pre", "preview":
			ver.Pre = "rc"
		}
		ver.PreNum, _ = strconv.Atoi(match[5])
	}

	// Post-release segment
	if match[6] != "" {
		ver.Post = "post"
		if match[8] == "" {
			ver.PostNum, _ = strconv.Atoi(match[7])
		} else {
			ver.PostNum, _ = strconv.Atoi(match[9])
		}
	}

	// Development release segment
	if match[10] != "" {
		ver.Dev = "dev"
		ver.DevNum, _ = strconv.Atoi(match[12])
	}

	// Local version label
	if match[13] != "" {
		ver.Local = strings.Split(match[13], ".")
	}

	if modify != nil {
		modify(&ver)
	}
	ver.Raw = ver.stringer()

	// ==================================
	// Adjust the Pre, Post, Dev type for comparison

	// Trick the sorting algorithm to put 1.0.dev0 before 1.0a0.
	// We'll do this by abusing the pre segment, but we _only_ want to do this
	// if there is not a pre or a post segment. If we have one of those then
	// the normal sorting rules will handle this case correctly.
	if ver.Pre == "" {
		if ver.Post == "" && ver.Dev != "" {
			ver.Pre = NegInfinity
		} else {
			ver.Pre = Infinity
		}
	}
	// Versions without a post segment should sort before those with one.
	if ver.Post == "" {
		ver.Post = NegInfinity
	}
	// Versions without a development segment should sort after those with one.
	if ver.Dev == "" {
		ver.Dev = Infinity
	}
	return
}

// return negative number when a < b
// return positive number when a > b
// return 0 when a == b
func (a Version) Compare(other repointerface.Version) (result int) {
	b, _ := other.(Version)
	result = a.Epoch - b.Epoch
	if result != 0 {
		return
	}
	result = slices.Compare(a.Release, b.Release)
	if result != 0 {
		return
	}
	result = strings.Compare(a.Pre, b.Pre)
	if result != 0 {
		return
	}
	result = a.PreNum - b.PreNum
	if result != 0 {
		return
	}
	result = strings.Compare(a.Post, b.Post)
	if result != 0 {
		return
	}
	result = a.PostNum - b.PostNum
	if result != 0 {
		return
	}
	result = strings.Compare(a.Dev, b.Dev)
	if result != 0 {
		return
	}
	result = a.DevNum - b.DevNum
	if result != 0 {
		return
	}
	return slices.CompareFunc(a.Local, b.Local, compareLocalPart)
}

func compareLocalPart(a, b string) (result int) {
	numA, err := strconv.Atoi(a)
	if err != nil {
		numA = -1
	}
	numB, err := strconv.Atoi(b)
	if err != nil {
		numB = -1
	}
	if numA == -1 && numB == -1 {
		return strings.Compare(a, b)
	}
	return cmp.Compare(numA, numB)
}
