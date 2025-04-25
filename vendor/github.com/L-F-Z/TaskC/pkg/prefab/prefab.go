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

package prefab

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
)

type Prefab struct {
	// DockerHub, PyPI, Apt, LOCAL
	SpecType string
	Name     string
	// "any" is the best practice, providing more flexibility for the scheduling system
	Specifier string
	// if Filters returns false, then this prefab is not needed
	Deployability *dcontext.Deployability
}

func (p *Prefab) String() string {
	result := "[" + p.SpecType + "] " + p.Name + " [" + p.Specifier + "]"
	if p.Deployability != nil {
		deployStr := p.Deployability.String()
		deployStr = strings.ReplaceAll(deployStr, `\`, `\\`)
		deployStr = strings.ReplaceAll(deployStr, `{`, `\{`)
		deployStr = strings.ReplaceAll(deployStr, `}`, `\}`)
		result += " {" + deployStr + "}"
	}
	return result
}

var deployRegex = regexp.MustCompile(`^(.*?)\s*(?:\{((?:\\.|[^{}\\])*)\})?$`)

func parsePrefab(str string) (p *Prefab, err error) {
	matches := deployRegex.FindStringSubmatch(str)
	if len(matches) == 0 {
		return nil, errors.New("string does not match prefab format")
	}

	first, middle, last, match := _parseBrackets(matches[1])
	if !match {
		return nil, errors.New("string does not match prefab format")
	}

	p = &Prefab{
		SpecType:  first,
		Name:      middle,
		Specifier: last,
	}

	if len(matches) > 2 && matches[2] != "" {
		deployStr := unescapeString(matches[2])
		p.Deployability, err = dcontext.ParseDeployability(deployStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse deployability %s: [%v]", matches[4], err)
		}
	}
	return p, nil
}

func _parseBrackets(s string) (first string, middle string, last string, match bool) {
	s = strings.TrimSpace(s)
	if s[0] != '[' {
		return
	}
	firstEnd := strings.Index(s, "]")
	if firstEnd == -1 {
		return
	}
	first = s[1:firstEnd]

	lastEnd := len(s) - 1
	if s[lastEnd] != ']' {
		return
	}
	lastStart := strings.LastIndex(s, "[")
	if lastStart == -1 {
		return
	}
	last = s[lastStart+1 : lastEnd]

	middle = strings.TrimSpace(s[firstEnd+1 : lastStart])
	match = true
	return
}

func unescapeString(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			result.WriteByte(s[i])
		} else {
			result.WriteByte(s[i])
		}
		i++
	}
	return result.String()
}

// Generate Prefab file and Blueprint file
func Pack(srcDir string, dstDir string, blueprint Blueprint) (prefabPath string, blueprintPath string, err error) {
	err = os.MkdirAll(dstDir, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("unable to create the directory to store prefab and blueprint files: %v", err)
		return
	}

	blueprintFilename := utils.SafeFilename(dstDir, ".blueprint", blueprint.Name, blueprint.Version, blueprint.Environment)
	blueprintPath = filepath.Join(dstDir, blueprintFilename)
	encoded, err := blueprint.encode()
	if err != nil {
		err = fmt.Errorf("unable to encode blueprint: %v", err)
		return
	}
	err = os.WriteFile(blueprintPath, []byte(encoded), os.ModePerm)
	if err != nil {
		err = fmt.Errorf("unable to save blueprint file: %v", err)
		return
	}

	prefabFilename := utils.SafeFilename(dstDir, ".prefab", blueprint.Name, blueprint.Version, blueprint.Environment)
	prefabPath = filepath.Join(dstDir, prefabFilename)
	err = packing.Pack(srcDir, dstDir, prefabFilename)
	if err != nil {
		err = fmt.Errorf("error occured when packing prefab: %v", err)
		return
	}
	return
}

func Unpack(prefabPath string, dstDir string) (err error) {
	return packing.Unpack(prefabPath, dstDir, ".prefab", "")
}
