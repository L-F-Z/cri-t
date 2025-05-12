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

package prefabservice

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

func (ps *PrefabService) RequestBlueprint(repoType string, name string, specifier repointerface.Constraint, ctx *dcontext.DeployContext) (blueprint *prefab.Blueprint, blueprintID string, prefabID string, err error) {
	var envSpec repointerface.EnvSpec
	repo, exists := ps.repos[repoType]
	if exists {
		err = repo.Init(ctx)
		if err != nil {
			return nil, "", "", fmt.Errorf("unable to init %s repo: [%v]", repoType, err)
		}
		envSpec = repo.GetEnvSpec()
	}
	specSheet := repointerface.SpecSheet{
		Type:      repoType,
		Name:      name,
		Specifier: specifier,
		EnvSpec:   envSpec,
	}

	prefabID, blueprintID, err = ps.PrefabSelection(specSheet)
	if err != nil {
		err = fmt.Errorf("unable to find a prefab for current request: [%v]", err)
		return
	}
	if blueprintID == "" {
		err = fmt.Errorf("unable to find a blueprint for current request %s-%s", name, specifier)
		return
	}

	path := ps.fileStore.genPath(blueprintID)
	var bytes []byte
	if utils.PathExists(path) {
		bytes, err = os.ReadFile(path)
		if err != nil {
			err = fmt.Errorf("failed to read local blueprint file: [%v]", err)
		}
	} else {
		// Get blueprint string, but not download it as a file
		var upstreamFile io.ReadCloser
		upstreamFile, _, _, err = ps.GetUpstreamFile(blueprintID)
		if err != nil {
			err = fmt.Errorf("failed to request upstream blueprint file: [%v]", err)
			return
		}
		defer upstreamFile.Close()
		bytes, err = io.ReadAll(upstreamFile)
		if err != nil {
			err = fmt.Errorf("failed to read upstream blueprint file: [%v]", err)
		}
	}

	bp, err := prefab.DecodeBlueprint(string(bytes))
	if err != nil {
		err = fmt.Errorf("unable to decode blueprint file: [%v]", err)
		return
	}
	blueprint = &bp
	return
}

func (ps *PrefabService) RequestPrefabBlueprint(blueprintID string, prefabID string) (blueprint prefab.Blueprint, prefabPath string, err error) {
	blueprintPath, err := ps.getBlueprintFile(blueprintID)
	if err != nil {
		err = fmt.Errorf("unable to get blueprint file from local Prefab Serivce: [%v]", err)
		return
	}
	blueprint, err = prefab.DecodeBlueprintFile(blueprintPath)
	if err != nil {
		err = fmt.Errorf("unable to decode blueprint file: [%v]", err)
		return
	}

	targetDir := "/"
	if blueprint.TargetDir != "" {
		targetDir = blueprint.TargetDir
	}
	prefabPath, err = ps.getPrefabUnpack(prefabID, targetDir)
	if err != nil {
		err = fmt.Errorf("unable to get prefab file from local Prefab Serivce: [%v]", err)
		return
	}
	err = ps.infoStore.SetItem(blueprint.Type, blueprint.Name, blueprint.Version, blueprint.Environment, prefabID, blueprintID)
	if err != nil {
		err = fmt.Errorf("failed to set info store item: [%v]", err)
		return
	}
	return
}

func (ps *PrefabService) WaitDownload(ids []string) error {
	return ps.fileStore.WaitDownload(ids)
}

func (ps *PrefabService) SizeSum(ids []string) (size uint64, err error) {
	for _, id := range ids {
		info, exist := ps.fileStore.files[id]
		if !exist {
			err = fmt.Errorf("prefab %s unfound", id)
		}
		size += info.FileSize
	}
	return
}

func (ps *PrefabService) RequestClosure(name string, version string, dstDir string) (closureName string, err error) {
	ver, err := ParseAnyVersion(repointerface.REPO_CLOSURE, version)
	if err != nil {
		return "", fmt.Errorf("failed to parse version %s: [%v]", version, err)
	}
	specSheet := repointerface.SpecSheet{
		Type:      repointerface.REPO_CLOSURE,
		Name:      name,
		Specifier: repointerface.SingleVersionConstraint(ver),
	}
	closureID, _, err := ps.PrefabSelection(specSheet)
	if err != nil {
		err = fmt.Errorf("error occured when selecting the task closure: [%v]", err)
		return
	}
	if closureID == "" {
		err = fmt.Errorf("unable to find a task closure for current request %s-%s", name, version)
		return
	}

	params := url.Values{}
	params.Add("id", closureID)
	fullURL := fmt.Sprintf("%s/file?%s", ps.upstream, params.Encode())

	closureName, err = utils.Download(fullURL, dstDir, "")
	return
}
