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
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

// The format of deb specifier is described in https://www.debian.org/doc/debian-policy/ch-relationships.html
var spec_regexp *regexp.Regexp = regexp.MustCompile(`^\s*(<<|<=|=|>=|>>)\s*(.*)\s*$`)

func DecodeSpecifier(specifier string) (c repointerface.Constraint, err error) {
	c = repointerface.AnyConstraint
	c.Raw = specifier
	c.RepoType = repointerface.REPO_APT
	specifier = strings.TrimSpace(specifier)
	if specifier == "any" || specifier == "latest" {
		return
	}

	for part := range bytes.SplitSeq([]byte(specifier), []byte(",")) {
		match := spec_regexp.FindSubmatch(part)
		if match == nil {
			err = errors.New("cannot match specifier " + specifier)
			return
		}
		var version Version
		version, err = ParseVersion(string(match[2]))
		if err != nil {
			err = fmt.Errorf("failed to parse version string %s: [%v]", match[2], err)
			return
		}
		var new repointerface.Constraint
		switch string(match[1]) {
		case "<<":
			new.AddRange(nil, version, false, false)
		case "<=":
			new.AddRange(nil, version, false, true)
		case "=":
			new.AddRange(version, version, true, true)
		case ">=":
			new.AddRange(version, nil, true, false)
		case ">>":
			new.AddRange(version, nil, false, false)
		}
		c = c.Intersect(new)
	}
	return
}
