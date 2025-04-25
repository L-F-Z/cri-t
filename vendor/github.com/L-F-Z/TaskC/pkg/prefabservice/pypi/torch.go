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
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
	mapset "github.com/deckarep/golang-set/v2"
)

var torch_pkg_names = []string{"torch", "torchvision", "torchaudio"}

func isTorchName(name string) bool {
	return slices.Contains(torch_pkg_names, name)
}

func isTorchVirtual(name string, version Version) bool {
	if slices.Contains(torch_pkg_names, name) {
		return len(version.Local) == 0
	}
	return false
}

func getTorchVersions(name string) (candidates []whlPackage, err error) {
	cands, err := torchGetCandidates(name)
	if err != nil {
		return
	}
	vers := mapset.NewSet[string]()
	for _, candidate := range cands {
		version := candidate.Version
		sep := strings.Index(version, "+")
		if sep != -1 {
			version = version[:sep]
			candidates = append(candidates, candidate)
		}
		vers.Add(version)
	}
	versSlice := vers.ToSlice()
	sort.Strings(versSlice)
	for _, ver := range versSlice {
		candidates = append(candidates, whlPackage{
			Name:    name,
			Version: ver,
			Env:     "py2.py3-none-any",
			Link:    "VIRTUAL",
		})
	}
	return
}

func fabricateTorchVirtual(name string, version string, dstDir string) (prefabPath string, blueprintPath string, err error) {
	cands, err := torchGetCandidates(name)
	if err != nil {
		return
	}
	envs := mapset.NewSet[string]()
	for _, candidate := range cands {
		ver := candidate.Version
		sep := strings.Index(ver, "+")
		if sep == -1 {
			continue
		}
		if ver[:sep] != version {
			continue
		}
		envs.Add(ver[sep+1:])
	}
	envsSlice := envs.ToSlice()
	sort.Strings(envsSlice)

	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_PYPI
	blueprint.Name = name
	blueprint.Version = version
	blueprint.Environment = "py2.py3-none-any"
	var deps []*prefab.Prefab
	for _, env := range envsSlice {
		var deployability *dcontext.Deployability
		if strings.HasPrefix(env, "cu") {
			// cu124 -> 12.4
			len := len(env)
			deployability = new(dcontext.Deployability)
			deployability.Add(dcontext.CUDA_TOOLKIT_VERSION, env[2:len-1]+"."+env[len-1:])
		} else if strings.HasPrefix(env, "rocm") {
			deployability = new(dcontext.Deployability)
			deployability.Add(dcontext.AMD_ROCM_VERSION, env[4:])
		} else if env == "cpu" {
			// Do nothing here
		} else {
			// Ignore other envs
			continue
		}

		deps = append(deps, &prefab.Prefab{
			SpecType:      repointerface.REPO_PYPI,
			Name:          name,
			Specifier:     "===" + version + "+" + env,
			Deployability: deployability,
		})
	}
	blueprint.Depend = [][]*prefab.Prefab{deps}
	emptyDir, err := os.MkdirTemp("", "")
	if err != nil {
		return
	}
	defer os.RemoveAll(emptyDir)
	return prefab.Pack(emptyDir, dstDir, blueprint)
}

const TORCH_BASE_URL = "https://download.pytorch.org/"

func torchGetCandidates(name string) (candidates []whlPackage, err error) {
	url := utils.CombineURL(TORCH_BASE_URL, "whl", name)
	body, _, err := utils.HttpGet(url)
	if err != nil {
		err = fmt.Errorf("error occured when requesting Simple API : %v", err)
		return
	}
	reg := regexp.MustCompile(`<a href="([^"]*)"[^>]*>([^<]*\.whl)`)
	files := reg.FindAllStringSubmatch(string(body), -1)

	pattern := `^([^\s-]+?)-([^\s-]*?)(-(\d[^-]*?))?-([^\s-]+?)-([^\s-]+?)-([^\s-]+?)\.whl$`
	pkg_regexp := regexp.MustCompile(pattern)
	for _, file := range files {
		match := pkg_regexp.FindStringSubmatch(file[2])
		if match == nil {
			fmt.Println(file[2] + " is not a valid whl file name string, ignored")
			continue
		}
		if len(match[2]) > len(".with.pypi.cudnn") && strings.HasSuffix(match[2], ".with.pypi.cudnn") {
			continue
		}
		candidates = append(candidates, whlPackage{
			Name:    match[1],
			Version: match[2],
			Env:     match[5] + "-" + match[6] + "-" + match[7], // pyVers-ABIs-platforms
			Link:    utils.CombineURL(TORCH_BASE_URL, file[1]),
		})
	}
	return
}
