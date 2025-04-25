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

package dcontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const PYTHON_VERSION_KEY = "os.pythonVersion"
const PYTHON_BIN_PATH = "os.pythonBinPath"

func init() {
	DeployabilityEvaluators[PYTHON_VERSION_KEY] = PythonVersionEvaluator
}

// specifier should be like `>=3.5`, `< 3.11.3`, ` ~=3.0`
// minor version number is required
// patch version number is optional
func PythonVersionEvaluator(specifier string, dc *DeployContext) (int, error) {
	// TODO: Follow https://peps.python.org/pep-0508/
	value, exists := dc.Get(PYTHON_VERSION_KEY)
	if !exists {
		return 0, fmt.Errorf("key %s not found in deployment context", PYTHON_VERSION_KEY)
	}
	pythonVersion, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("received python version context value is not a string")
	}
	return EvaluatePythonVersion(specifier, pythonVersion)
}

func EvaluatePythonVersion(specifier string, pythonVersion string) (int, error) {
	parts := strings.Split(pythonVersion, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("python version in context is [%s], but should be in X.X format", pythonVersion)
	}
	aMajor, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("unable to decode %s.%s, not a valid number", parts[0], parts[1])
	}
	aMinor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("unable to decode %s.%s, not a valid number", parts[0], parts[1])
	}

	pyVerSpecifier := regexp.MustCompile(`(>=|<=|==|>|<|~=|!=)\s*(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	matches := pyVerSpecifier.FindStringSubmatch(specifier)

	operator := matches[1]
	bMajor, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, fmt.Errorf("unable to decode major version mumber %s, not a valid number", matches[2])
	}
	bMinor := 0
	if matches[3] != "" {
		bMinor, err = strconv.Atoi(matches[3])
		if err != nil {
			return 0, fmt.Errorf("unable to decode minor version number %s, not a valid number", matches[3])
		}
	}
	// bPatch := 0
	// if matches[4] != "" {
	// 	bPatch, err = strconv.Atoi(matches[4])
	// 	if err != nil {
	// 		return 0, fmt.Errorf("unable to decode patch number %s, not a valid number", matches[4])
	// 	}
	// }

	var result int
	if aMajor != bMajor {
		result = aMajor - bMajor
	} else if aMinor != bMinor {
		result = aMinor - bMinor
	}
	// currently we ignore Patch
	//  else {
	// 	result = -bPatch
	// }

	var satisfied bool
	switch operator {
	case ">=":
		satisfied = result >= 0
	case ">":
		satisfied = result > 0
	case "<=":
		satisfied = result <= 0
	case "<":
		satisfied = result < 0
	case "==":
		satisfied = result == 0
	case "!=":
		satisfied = result != 0
	case "~=":
		satisfied = aMajor == bMajor && aMinor >= bMinor
	}
	if satisfied {
		return 255, nil
	} else {
		return 0, nil
	}
}

func (d *DeployContext) SetPythonVersion(root string) (err error) {
	major, minor, err := PythonVersion(root)
	if err != nil {
		return fmt.Errorf("unable to get python version information: [%v]", err)
	}
	err = d.Set(PYTHON_VERSION_KEY, fmt.Sprintf("%d.%d", major, minor))
	if err != nil {
		return fmt.Errorf("unable to set python version context: [%v]", err)
	}
	return
}

// Get python version [major.minor]
func PythonVersion(root string) (major int, minor int, err error) {
	binPath, err := PythonBinPath(root)
	if err != nil {
		return
	}
	re := regexp.MustCompile(`python(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(filepath.Base(binPath))
	if matches == nil || len(matches) != 3 {
		return 0, 0, errors.New("file name does not match pythonX.Y format")
	}

	major, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, err
	}

	minor, err = strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, err
	}

	return major, minor, nil
}

func (d *DeployContext) SetPythonBinPath(root string) (err error) {
	binPath, err := PythonBinPath(root)
	if err != nil {
		return fmt.Errorf("unable to get python bin path information: [%v]", err)
	}
	err = d.Set(PYTHON_BIN_PATH, binPath)
	if err != nil {
		return fmt.Errorf("unable to set python bin path context: [%v]", err)
	}
	return
}

func PythonBinPath(root string) (binPath string, err error) {
	paths := []string{
		"usr/local/bin/python3",
		"usr/local/bin/python",
		"usr/bin/python3",
		"usr/bin/python",
	}
	for _, path := range paths {
		joinedPath := filepath.Join(root, path)
		if _, err = os.Stat(joinedPath); err == nil {
			binPath, err = filepath.EvalSymlinks(joinedPath)
			if err == nil {
				relativePath, err := filepath.Rel(root, binPath)
				if err == nil {
					if !filepath.IsAbs(relativePath) {
						relativePath = "/" + relativePath
					}
					return relativePath, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no Python binary found")
}
