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
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
)

type PyPIQuery struct {
	Releases map[string][]Package `json:"releases"`
}

type Package struct {
	PackageType    string  `json:"packagetype"`
	RequiresPython *string `json:"requires_python"`
	URL            string  `json:"url"`
}

func getCandidates(name string) (candidates []whlPackage, err error) {
	if isTorchName(name) {
		return getTorchVersions(name)
	}
	url := utils.CombineURL("https://pypi.org/pypi/", name) + "/json/"
	body, _, err := utils.HttpGet(url)
	if err != nil {
		err = fmt.Errorf("error occured when requesting PyPI API : %v", err)
		return
	}
	var query PyPIQuery
	err = json.Unmarshal(body, &query)
	if err != nil {
		log.Fatalf("Error unmarshaling JSON: %v", err)
	}

	whlPattern := regexp.MustCompile(`^([^\s-]+?)-([^\s-]*?)(-(\d[^-]*?))?-([^\s-]+?)-([^\s-]+?)-([^\s-]+?)\.whl$`)
	for _, packages := range query.Releases {
		for _, pkg := range packages {
			switch pkg.PackageType {
			case "bdist_wheel":
				filename := filepath.Base(pkg.URL)
				match := whlPattern.FindStringSubmatch(filename)
				if match == nil {
					log.Println(filename + " is not a valid whl file name string, ignored")
					continue
				}

				envStr := match[5] + "-" + match[6] + "-" + match[7] // pyVers-ABIs-platforms
				if pkg.RequiresPython != nil {
					envStr = "#" + requiresPythonToEnv(*pkg.RequiresPython) + "#" + envStr
				}
				// match[3] & match[4] is build info, ignore them
				candidates = append(candidates, whlPackage{
					Name:    name,
					Version: match[2],
					Env:     envStr,
					Link:    pkg.URL,
				})
			case "sdist":
				filename := filepath.Base(pkg.URL)
				lowerName := strings.ToLower(filename)
				if strings.HasSuffix(lowerName, ".tar.gz") {
					filename = filename[0 : len(filename)-len(".tar.gz")]
				} else if strings.HasSuffix(lowerName, ".zip") {
					filename = filename[0 : len(filename)-len(".zip")]
				} else if strings.HasSuffix(lowerName, ".tgz") {
					filename = filename[0 : len(filename)-len(".tgz")]
				} else {
					log.Println("source package not in `.tar.gz`, `.tgz` or `.zip` format", filename)
					continue
				}

				versionSeparator := strings.LastIndex(filename, "-")
				if versionSeparator == -1 {
					log.Printf("source package name %s is not in `name-version.tar.gz` format\n", filename)
					continue
				}

				envStr := SOURCE_DISTRIBUTION_ENV_TAG
				if pkg.RequiresPython != nil {
					envStr = "#" + requiresPythonToEnv(*pkg.RequiresPython) + "#" + envStr
				}

				candidates = append(candidates, whlPackage{
					Name:    name,
					Version: filename[versionSeparator+1:],
					Env:     envStr,
					Link:    pkg.URL,
				})
			default:
				continue
			}

		}
	}
	return
}

func requiresPythonToEnv(requiresPython string) (envSpecifier string) {
	parts := strings.Split(requiresPython, ",")
	var transformed []string
	replacer := strings.NewReplacer(" ", "", ">", "g", "=", "e", "<", "l", "!", "n")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		replaced := replacer.Replace(trimmed)
		transformed = append(transformed, replaced)
	}
	return strings.Join(transformed, "-")
}

func envToRequiresPython(envSpecifier string) (reqs []string) {
	reqs = make([]string, 0)
	if envSpecifier == "" {
		return
	}
	parts := strings.Split(envSpecifier, "-")
	replacer := strings.NewReplacer("g", ">", "e", "=", "l", "<", "n", "!")
	for _, part := range parts {
		replaced := replacer.Replace(part)
		reqs = append(reqs, replaced)
	}
	return
}

func isSourceDist(env string) bool {
	pure, _, _ := getRequiresPython(env)
	return pure == SOURCE_DISTRIBUTION_ENV_TAG
}
