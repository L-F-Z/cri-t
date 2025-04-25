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
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type EnvSpec struct {
	PyVer   string `json:"pyVer"`   // e.g. "3.10"
	LibcVer string `json:"libcVer"` // e.g. "2.36"
	Arch    string `json:"arch"`    // e.g. "amd64"
}

func (es EnvSpec) Encode() string {
	s, _ := json.Marshal(es)
	return string(s)
}

func DecodeEnvSpec(s string) (es EnvSpec, err error) {
	err = json.Unmarshal([]byte(s), &es)
	return
}

func (r *Repo) SelectVersion(versions []repointerface.Version) (selected repointerface.Version, err error) {
	vers := make([]Version, 0, len(versions))
	for _, version := range versions {
		if ver, ok := version.(Version); ok {
			vers = append(vers, ver)
		} else {
			return nil, fmt.Errorf("version %v has wrong type", version)
		}
	}
	if len(vers) == 0 {
		return
	}
	// igonore Pre-Release and Local versions
	for _, ver := range vers {
		if ver.Pre != "" || ver.Local != nil {
			continue
		}
		selected = ver
		break
	}
	if selected != nil {
		return
	}

	// Try Pre-Release
	for _, ver := range vers {
		if ver.Local != nil {
			continue
		}
		selected = ver
		break
	}
	if selected == nil {
		// Use Local
		selected = vers[0]
	}
	return
}

const SOURCE_DISTRIBUTION_ENV_TAG = "sdist"

func (r *Repo) SelectEnv(envs []string, envSpec repointerface.EnvSpec) (selected string, err error) {
	spec, ok := envSpec.(EnvSpec)
	if !ok {
		return "", errors.New("mismatch envSpec type")
	}
	sourceDist := ""
	candidates := make([]pkgEnv, 0)
	for _, env := range envs {
		original, pyReqs, err := getRequiresPython(env)
		if err != nil {
			fmt.Println(err)
			continue
		}

		deployable := true
		for _, req := range pyReqs {
			deployability, err := dcontext.EvaluatePythonVersion(req, spec.PyVer)
			if err != nil {
				log.Printf("unable to evaluate deployability for %s: [%v]\n", env, err)
				deployable = false
				continue
			}
			if deployability <= 0 {
				deployable = false
			}
		}
		if deployable {
			if original == SOURCE_DISTRIBUTION_ENV_TAG {
				sourceDist = env
				continue
			}
			decoded, err := decodeEnv(env)
			if err != nil {
				fmt.Println(err)
				continue
			}
			candidates = append(candidates, decoded)
		}
	}
	candidates = selectPyVerAbis(candidates, spec.PyVer)
	selected = selectPlatform(candidates, spec.Arch, spec.LibcVer).str
	if selected == "" {
		selected = sourceDist
	}
	return
}

func (r *Repo) FilterEnv(envs []string) (selected []string) {
	source := ""
	for _, env := range envs {
		if strings.Contains(env, "win") {
			continue
		}
		if strings.Contains(env, "macosx") {
			continue
		}
		if strings.Contains(env, "musllinux") {
			continue
		}
		if strings.Contains(env, "pypy") {
			continue
		}
		if isSourceDist(env) {
			source = env
			continue
		}
		selected = append(selected, env)
	}
	if len(selected) == 0 && source != "" {
		selected = append(selected, source)
	}
	return
}

type pkgEnv struct {
	pyVerAbis  []string
	platforms  []string
	requiresPy []string
	str        string
}

func getRequiresPython(environment string) (remain string, pyReqs []string, err error) {
	requiresPython := ""
	remain = environment
	if environment[0] == '#' {
		index := strings.Index(environment[1:], "#")
		if index == -1 {
			err = errors.New("environment start with # but cannot find another one")
			return
		}
		requiresPython = environment[1 : index+1]
		remain = environment[index+2:]
	}
	pyReqs = envToRequiresPython(requiresPython)
	return
}

func decodeEnv(environment string) (p pkgEnv, err error) {
	original, pyReqs, err := getRequiresPython(environment)
	if err != nil {
		return
	}

	parts := strings.Split(original, "-")
	if len(parts) != 3 {
		err = errors.New("unable to decode environment " + original)
		return
	}
	pyVers := strings.Split(parts[0], ".")
	abis := strings.Split(parts[1], ".")
	platforms := strings.Split(parts[2], ".")
	var pyVerAbis []string
	for _, pyVer := range pyVers {
		for _, abi := range abis {
			pyVerAbis = append(pyVerAbis, pyVer+"-"+abi)
		}
	}
	return pkgEnv{
		pyVerAbis:  pyVerAbis,
		platforms:  platforms,
		requiresPy: pyReqs,
		str:        environment,
	}, nil
}
