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
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

// TODO: Decode name, version and environment from package METADATA and WHEEL
func Fabricate(pkgPath string, name string, version string, environment string, dstDir string) (prefabPath string, blueprintPath string, err error) {
	tmpDir, err := os.MkdirTemp("", "WhlFabricate-")
	if err != nil {
		err = fmt.Errorf("error occured when create temp directory for fabracation: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	_, features := getFeatures(name)
	var requiredFeature string = ""
	if len(features) > 1 {
		err = errors.New("cannot require multiple features in one prefab: " + name)
		return
	}
	if len(features) == 1 {
		requiredFeature = features[0]
	}

	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_PYPI
	blueprint.Name = name
	blueprint.Version = version
	blueprint.Environment = environment
	blueprint.TargetDir = "/usr/local/lib/python-site-packages"

	err = packing.Unpack(pkgPath, tmpDir, ".zip", "")
	if err != nil {
		err = fmt.Errorf("error occured when unpacking whl: %v", err)
		return
	}

	distInfoDir, err := getDistInfoDir(tmpDir)
	if err != nil {
		err = fmt.Errorf("error occured when finding .dist-info directory: %v", err)
		return
	}

	metadataPath := filepath.Join(distInfoDir, "METADATA")
	if !utils.PathExists(metadataPath) {
		err = errors.New("unable to find MATADATA in " + metadataPath)
		return
	}

	file, err := os.Open(metadataPath)
	if err != nil {
		err = fmt.Errorf("error occured when opening METADATA file: %v", err)
		return
	}
	defer file.Close()

	// https://packaging.python.org/en/latest/specifications/core-metadata/#requires-dist-multiple-use
	getSpecifier := regexp.MustCompile(`^Requires-Dist:\s*(.*)`)
	getRequiresPython := regexp.MustCompile(`^Requires-Python:\s*(.*)`)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := getSpecifier.FindStringSubmatch(line)
		if matches != nil {
			err = analyseDepend(matches[1], requiredFeature, &blueprint)
			if err != nil {
				return
			}
		}
		matches = getRequiresPython.FindStringSubmatch(line)
		if matches != nil {
			blueprint.Environment, err = requiresPython(matches[1], environment)
			if err != nil {
				return
			}
		}
	}
	if scanner.Err() != nil {
		err = fmt.Errorf("error occured when scanning METADATA file: %v", scanner.Err())
	}
	return prefab.Pack(tmpDir, dstDir, blueprint)
}

func analyseDepend(input string, requiredFeature string, blueprint *prefab.Blueprint) (err error) {
	deployability := new(dcontext.Deployability)
	parts := strings.Split(input, ";")
	// refer to https://packaging.python.org/en/latest/specifications/dependency-specifiers/
	isExtra := regexp.MustCompile(`extra\s*==\s*["']([^"']+)["']`)
	isPlatformSystem := regexp.MustCompile(`platform_system\s*==\s*["']([^"']+)["']`)
	isSysPlatform := regexp.MustCompile(`sys_platform\s*==\s*["']([^"']+)["']`)
	isOsName := regexp.MustCompile(`os_name\s*==\s*["']([^"']+)["']`)
	isPythonVersion := regexp.MustCompile(`python_version\s*(>=|<=|==|>|<|~=|!=)\s*["']?(\d+)(\.\d+)?["']?`)
	for i := 1; i < len(parts); i++ {
		match := isExtra.FindStringSubmatch(strings.TrimSpace(parts[i]))
		if len(match) > 1 {
			if requiredFeature == "" {
				return
			}
			if requiredFeature != match[1] {
				return
			}
		}
		match = isPlatformSystem.FindStringSubmatch(strings.TrimSpace(parts[i]))
		if len(match) > 1 {
			if match[1] != "Linux" {
				return
			}
		}
		match = isSysPlatform.FindStringSubmatch(strings.TrimSpace(parts[i]))
		if len(match) > 1 {
			if match[1] != "linux" {
				return
			}
		}
		match = isOsName.FindStringSubmatch(strings.TrimSpace(parts[i]))
		if len(match) > 1 {
			if match[1] != "posix" {
				return
			}
		}
		match = isPythonVersion.FindStringSubmatch(strings.TrimSpace(parts[i]))
		if len(match) > 3 {
			if match[3] == "" {
				deployability.Add(dcontext.PYTHON_VERSION_KEY, match[1]+match[2]+".0")
			} else {
				deployability.Add(dcontext.PYTHON_VERSION_KEY, match[1]+match[2]+match[3])
			}
		}
	}
	dependency := strings.TrimSpace(parts[0])
	dependency = strings.ReplaceAll(dependency, "(", "")
	dependency = strings.ReplaceAll(dependency, ")", "")
	name, condition := splitDependency(dependency)
	// we decode it and encode it to ensure the format is correct
	_, err = DecodeSpecifier(condition)
	if err != nil {
		return fmt.Errorf("failed to decode dependency specifier %s: [%v]", condition, err)
	}

	dependNames := splitFeatures(name)
	if len(*deployability) == 0 {
		for _, name := range dependNames {
			blueprint.AddDepend(&prefab.Prefab{
				SpecType:  repointerface.REPO_PYPI,
				Name:      NameNormalizer(name),
				Specifier: condition,
			})
		}
	} else {
		for _, name := range dependNames {
			blueprint.AddDepend(&prefab.Prefab{
				SpecType:      repointerface.REPO_PYPI,
				Name:          NameNormalizer(name),
				Specifier:     condition,
				Deployability: deployability,
			})
		}
	}
	return
}

func requiresPython(input string, env string) (newEnv string, err error) {
	newEnv = env
	_, pyReqs, _ := getRequiresPython(env)
	if len(pyReqs) > 0 {
		// the version requirements info provided by PyPI is more accurate than the one in package's METADATA
		// so no change needed
		return
	}

	rePythonVersion := regexp.MustCompile(`(>=|<=|==|>|<|~=|!=)\s*["']?(\d+)(\.\d+){0,2}["']?`)
	match := rePythonVersion.FindStringSubmatch(strings.TrimSpace(input))
	if len(match) == 0 {
		err = fmt.Errorf("unable to match Requires-Python part %s", input)
		return
	}
	condition := match[1]
	majorVersion := match[2]
	minorVersion := ".0"
	if len(match) > 3 && match[3] != "" {
		minorVersion = match[3]
	}
	result := condition + majorVersion + minorVersion

	newEnv = "#" + requiresPythonToEnv(result) + "#" + env
	return
}

func splitDependency(dependency string) (name string, condition string) {
	var splitIndex int = -1
	for i, ch := range dependency {
		if ch == '<' || ch == '>' || ch == '=' || ch == '!' || ch == '~' {
			splitIndex = i
			break
		}
	}

	if splitIndex == -1 {
		name = dependency
		condition = "any"
	} else {
		name = strings.TrimSpace(dependency[:splitIndex])
		condition = dependency[splitIndex:]
	}
	return
}

func getDistInfoDir(rootDir string) (string, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", rootDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".dist-info") {
			return filepath.Join(rootDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no .dist-info directory found in %s", rootDir)
}
