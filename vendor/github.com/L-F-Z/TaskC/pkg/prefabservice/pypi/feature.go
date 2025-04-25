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
	"regexp"
	"strings"
)

var featurePattern = regexp.MustCompile(`([^[]+)(?:\[([^\]]+)\])?`)

func splitFeatures(input string) (nameWithfeature []string) {
	matches := featurePattern.FindSubmatch([]byte(input))
	if len(matches) >= 3 {
		pureName := string(matches[1])
		for feature := range bytes.SplitSeq([]byte(matches[2]), []byte(",")) {
			feature = bytes.TrimSpace(feature)
			if len(feature) == 0 {
				nameWithfeature = append(nameWithfeature, pureName)
			} else {
				nameWithfeature = append(nameWithfeature, pureName+"["+string(feature)+"]")
			}
		}
	}
	return
}

func getFeatures(input string) (pureName string, features []string) {
	matches := featurePattern.FindStringSubmatch(input)
	if len(matches) > 1 {
		pureName = matches[1]
	}
	if len(matches) > 2 {
		features := strings.Split(matches[2], ",")
		for _, feature := range features {
			feature = strings.TrimSpace(feature)
			features = append(features, feature)
		}
	}
	return
}
