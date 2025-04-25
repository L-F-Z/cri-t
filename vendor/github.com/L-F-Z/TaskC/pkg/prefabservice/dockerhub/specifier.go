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

package dockerhub

import (
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type Version string

func (v Version) String() string {
	return string(v)
}

func ParseVersion(version string) (ver Version, err error) {
	return Version(version), nil
}

func (a Version) Compare(other repointerface.Version) (result int) {
	b, _ := other.(Version)
	return strings.Compare(string(a), string(b))
}

func DecodeSpecifier(specifier string) (c repointerface.Constraint, err error) {
	c.Raw = specifier
	c.RepoType = repointerface.REPO_DOCKERHUB
	specifier = strings.TrimSpace(specifier)
	if specifier == "any" || specifier == "latest" {
		c.AddRange(nil, nil, false, false)
	} else {
		ver := Version(specifier)
		c.AddRange(ver, ver, true, true)
	}
	return
}
