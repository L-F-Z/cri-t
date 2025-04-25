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
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

// https://peps.python.org/pep-0508/
var spec_regexp *regexp.Regexp = regexp.MustCompile(`^\s*(===|~=|==|!=|<=|>=|<|>)\s*(.*)\s*$`)

func DecodeSpecifier(specifier string) (c repointerface.Constraint, err error) {
	c = repointerface.AnyConstraint
	c.Raw = specifier
	c.RepoType = repointerface.REPO_PYPI
	specifier = strings.TrimSpace(specifier)
	if specifier == "any" || specifier == "latest" {
		return
	}
	for part := range bytes.SplitSeq([]byte(specifier), []byte(",")) {
		spec := bytes.TrimSpace(part)
		match := spec_regexp.FindSubmatch(spec)
		if match == nil {
			err = errors.New("cannot match specifier " + specifier)
			return
		}
		op := string(match[1])
		verRaw := match[2]
		var new repointerface.Constraint
		var version Version
		if bytes.HasSuffix(verRaw, []byte(".*")) { // has wildcard
			verRaw = bytes.TrimSuffix(verRaw, []byte(".*"))
			version, err = ParseVersion(string(verRaw))
			if err != nil {
				err = fmt.Errorf("cannot decode the version %s", string(verRaw))
				return
			}
			// 1.1.* means [1.1.dev0, 1.2.dev0)
			var upper Version
			// 1.1 -> 1.2
			upper, err = parseVersion(string(verRaw), func(v *Version) { v.Release[len(v.Release)-1]++ })
			if err != nil {
				err = fmt.Errorf("cannot decode the version %s", string(verRaw))
				return
			}
			version.Dev = "dev"
			upper.Dev = "dev"
			if op == "==" {
				new.AddRange(version, upper, true, false)
			} else if op == "!=" {
				new.AddRange(nil, version, false, false)
				new.AddRange(upper, nil, true, false)
			} else {
				err = errors.New(specifier + " is not valid, Wildcard only supports == and !=")
				return
			}
			c = c.Intersect(new)
			continue
		}

		version, err = ParseVersion(string(verRaw))
		if err != nil {
			err = fmt.Errorf("cannot decode the version %s", string(verRaw))
			return
		}
		switch op {
		case "===":
			new.AddRange(version, version, true, true)
		case "==":
			if len(version.Local) != 0 {
				new.AddRange(version, version, true, true)
			} else {
				localmax, _ := parseVersion(string(verRaw), func(v *Version) { v.Local = []string{strconv.Itoa(math.MaxInt)} })
				new.AddRange(version, localmax, true, true)
			}
		case "!=":
			new.AddRange(nil, version, false, false)
			new.AddRange(version, nil, false, false)
		case "<=":
			new.AddRange(nil, version, false, true)
		case "<":
			new.AddRange(nil, version, false, false)
		case ">=":
			new.AddRange(version, nil, true, false)
		case ">":
			new.AddRange(version, nil, false, false)
		case "~=":
			if len(version.Local) > 0 {
				err = errors.New(string(part) + " is not valid, local version label is not permitted")
			}
			var upper Version
			// 2.4.3 -> 2.5
			upper, err = parseVersion(string(verRaw), func(v *Version) {
				v.Release = v.Release[:len(v.Release)-1]
				v.Release[len(v.Release)-1]++
			})
			if err != nil {
				err = fmt.Errorf("failed to decode version and remove last Release number %s, ~= needs at least a minor release number", string(verRaw))
				return
			}
			new.AddRange(version, upper, true, false)
		}
		c = c.Intersect(new)
	}
	return
}
