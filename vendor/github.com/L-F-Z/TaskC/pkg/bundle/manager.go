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
	"sync"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice"
)

type BundleName struct {
	Name    string
	Version string
}

type BundleID string

type Bundle struct {
	Prefabs      map[string]string // Prefab Name -> Prefab Specifier
	PrefabIndexs map[string]int    // Prefab Name -> PrefabPaths Index
	PrefabPaths  []string
	PrefabIDs    []string
	RootFS       string
	LocalDir     string
	LocalDirCnt  int
	BasePath     string
	Blueprint    *prefab.Blueprint
}

const SPEC_NAME = "bundle.json"
const LIST_NAME = "Bundles.json"

type BundleManager struct {
	prefabService *prefabservice.PrefabService
	bundleDir     string
	bundles       map[string]map[string]string
	listPath      string
	sync.RWMutex
}

const perm = os.FileMode(0700)

func NewBundleManager(workDir string, upstream string) (bm *BundleManager, err error) {
	bm = &BundleManager{}
	bm.bundleDir = filepath.Join(workDir, "Bundle")
	err = os.MkdirAll(bm.bundleDir, perm)
	if err != nil {
		err = fmt.Errorf("unable to make dir %s [%v]", bm.bundleDir, err)
		return
	}

	bm.prefabService, err = prefabservice.NewUserService(workDir, upstream)
	if err != nil {
		err = fmt.Errorf("unable to create local prefab service: [%v]", err)
		return
	}

	// load exists names
	bm.bundles = make(map[string]map[string]string)
	bm.listPath = filepath.Join(bm.bundleDir, LIST_NAME)
	if !utils.PathExists(bm.listPath) {
		return
	}
	data, err := os.ReadFile(bm.listPath)
	if err != nil {
		err = fmt.Errorf("unable to read saved bundle list data: [%v]", err)
		return
	}
	err = json.Unmarshal(data, &bm.bundles)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal saved bundle list data: [%v]", err)
		return
	}
	return
}

func (bm *BundleManager) saveData() (err error) {
	data, err := json.MarshalIndent(bm.bundles, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bundle list: [%v]", err)
	}
	err = os.WriteFile(bm.listPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write bundle list file: [%v]", err)
	}
	return
}

func (bm *BundleManager) GetById(id string) (bundle *Bundle, err error) {
	bm.RLock()
	defer bm.RUnlock()
	specPath := filepath.Join(bm.bundleDir, id, SPEC_NAME)
	data, err := os.ReadFile(specPath)
	if err != nil {
		err = fmt.Errorf("unable to read bundle spec: [%v]", err)
		return
	}
	bundle = &Bundle{}
	err = json.Unmarshal(data, bundle)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal bundle spec: [%v]", err)
		return
	}
	return
}

func (bm *BundleManager) Get(name string, version string) (bundle *Bundle, err error) {
	bm.RLock()
	defer bm.RUnlock()
	id, exists := bm.getBundleID(name, version)
	if !exists {
		err = fmt.Errorf("bundle %s (%s) not exists", name, version)
		return
	}
	specPath := filepath.Join(bm.bundleDir, id, SPEC_NAME)
	data, err := os.ReadFile(specPath)
	if err != nil {
		err = fmt.Errorf("unable to read bundle spec: [%v]", err)
		return
	}
	bundle = &Bundle{}
	err = json.Unmarshal(data, bundle)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal bundle spec: [%v]", err)
		return
	}
	return
}

func (bm *BundleManager) AddBundleID(name string, version string, bundleID string) (err error) {
	bm.Lock()
	defer bm.Unlock()
	_, exists := bm.bundles[name]
	if !exists {
		bm.bundles[name] = make(map[string]string)
	}
	_, exists = bm.bundles[name][version]
	if exists {
		err = fmt.Errorf("bundle %s (%s) already exists", name, version)
		return
	}
	bm.bundles[name][version] = bundleID
	err = bm.saveData()
	return
}

func (bm *BundleManager) List() (bundles []string) {
	bm.RLock()
	defer bm.RUnlock()
	for name, versions := range bm.bundles {
		for version := range versions {
			bundles = append(bundles, fmt.Sprintf("%s (%s)", name, version))
		}
	}
	return
}

func (bm *BundleManager) Exist(name, version string) (exists bool) {
	bm.RLock()
	defer bm.RUnlock()
	_, exists = bm.getBundleID(name, version)
	return
}

func (bm *BundleManager) getBundleID(name, version string) (id string, exists bool) {
	_, exists = bm.bundles[name]
	if !exists {
		return
	}
	id, exists = bm.bundles[name][version]
	return
}

func (bm *BundleManager) Upload(repoType string, taskcPath string, blueprintPath string) (err error) {
	return bm.prefabService.PostUpload(repoType, taskcPath, blueprintPath)
}

func (bm *BundleManager) RequestClosure(name string, version string, dstDir string) (filename string, err error) {
	return bm.prefabService.RequestClosure(name, version, dstDir)
}
