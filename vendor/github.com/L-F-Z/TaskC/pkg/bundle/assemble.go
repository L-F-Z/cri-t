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

package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/L-F-Z/TaskC/pkg/bundle/pubgrub"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/google/uuid"
)

// assemble the blueprint into a given bundle
// blueprintPath must be an absoulute path
func (bm *BundleManager) Assemble(blueprint prefab.Blueprint, basePath string, dctx *dcontext.DeployContext) (err error) {
	bundleID := uuid.New().String()
	if err != nil {
		err = fmt.Errorf("unable to create a new bundle ID: [%v]", err)
		return
	}
	workDir := filepath.Join(bm.bundleDir, bundleID)
	err = os.MkdirAll(workDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to create work directory: [%v]", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(workDir)
		}
	}()

	bundle := &Bundle{
		Prefabs:     make(map[string]string, 0),
		LocalDir:    filepath.Join(workDir, "local"),
		LocalDirCnt: 0,
		BasePath:    basePath,
		Blueprint:   &blueprint,
	}

	nonLocal := FilterNonLocal(blueprint.Depend)
	// fmt.Printf("\rAnalyzing %-40.40s", fmt.Sprintf("%s (%s)", blueprint.Name, blueprint.Version))
	result, dctx, err := pubgrub.Solve(bm.prefabService, blueprint.Type, blueprint.Name, blueprint.Version, nonLocal, dctx)
	if err != nil {
		return fmt.Errorf("failed to solve version dependencies: [%v]", err)
	}
	dependency := make(map[string][]string)
	prefabPaths := make(map[string]string)
	for pkgName := range result {
		pkgInfo := result[pkgName]
		bp, prefabPath, err := bm.prefabService.RequestPrefabBlueprint(pkgInfo.BlueprintID, pkgInfo.PrefabID)
		if err != nil {
			return fmt.Errorf("failed to request %v prefab and blueprint: [%v]", pkgName, err)
		}
		prefabPaths[pkgName] = prefabPath
		bundle.PrefabIDs = append(bundle.PrefabIDs, pkgInfo.PrefabID)
		dependency[pkgName] = pkgInfo.Depends
		mergeBlueprint(bp, &blueprint)
	}

	// sort prefabPaths
	for _, alternatives := range nonLocal {
		for _, cand := range alternatives {
			pkgName := pubgrub.GenKey(cand.SpecType, cand.Name)
			addPath(pkgName, bundle, dependency, prefabPaths)
		}
	}

	err = bm.assembleLocal(bundle, &blueprint, dctx)
	if err != nil {
		return fmt.Errorf("failed to assemble blueprint %s: [%v]", blueprint.Name, err)
	}

	err = bm.prefabService.WaitDownload(bundle.PrefabIDs)
	if err != nil {
		return fmt.Errorf("error occured when downloading prefabs: [%v]", err)
	}

	bundle.RootFS, err = bm.mountOverlay(workDir, bundle.PrefabPaths)
	if err != nil {
		return fmt.Errorf("unable to mount overlay directory: [%v]", err)
	}

	specPath := filepath.Join(workDir, SPEC_NAME)
	file, err := os.Create(specPath)
	if err != nil {
		return fmt.Errorf("unable to create bundle spec: [%v]", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	err = encoder.Encode(bundle)
	return bm.AddBundleID(blueprint.Name, blueprint.Version, bundleID)
}

func FilterNonLocal(ori [][]*prefab.Prefab) (filtered [][]*prefab.Prefab) {
	for _, alternatives := range ori {
		var filterAlt []*prefab.Prefab
		for _, cand := range alternatives {
			if !slices.Contains(localTypes, cand.SpecType) {
				filterAlt = append(filterAlt, cand)
			}
		}
		if len(filterAlt) > 0 {
			filtered = append(filtered, filterAlt)
		}
	}
	return
}

func addPath(pkgName string, bundle *Bundle, dependency map[string][]string, prefabPaths map[string]string) {
	prefabPath, ok := prefabPaths[pkgName]
	if !ok {
		return
	}
	bundle.PrefabPaths = append(bundle.PrefabPaths, prefabPath)
	delete(prefabPaths, pkgName)
	for _, depend := range dependency[pkgName] {
		addPath(depend, bundle, dependency, prefabPaths)
	}
}

func mergeBlueprint(from prefab.Blueprint, to *prefab.Blueprint) {
	if to.User == "" && from.User != "" {
		to.User = from.User
	}
	if to.WorkDir == "" && from.WorkDir != "" {
		to.WorkDir = from.WorkDir
	}
	if len(to.EntryPoint) == 0 && len(from.EntryPoint) != 0 {
		to.EntryPoint = from.EntryPoint
	}
	if len(to.Command) == 0 && len(from.Command) != 0 {
		to.Command = from.Command
	}
	to.EnvVar = append(to.EnvVar, from.EnvVar...)
}
