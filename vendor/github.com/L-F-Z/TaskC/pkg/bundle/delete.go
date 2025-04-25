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
	"fmt"
	"os"
	"path/filepath"
)

func (bm *BundleManager) DeleteAllBundles() (err error) {
	bm.Lock()
	defer bm.Unlock()
	defer bm.saveData()
	for name := range bm.bundles {
		err = bm.deleteByName(name)
		if err != nil {
			return
		}
	}
	return
}

// if version == "", delete all versions of a bundle
func (bm *BundleManager) DeleteBundle(name string, version string) (err error) {
	bm.Lock()
	defer bm.Unlock()
	defer bm.saveData()
	if version == "" {
		err = bm.deleteByName(name)
	} else {
		err = bm.deleteByNameVersion(name, version)
	}
	return
}

func (bm *BundleManager) deleteByName(name string) (err error) {
	_, exists := bm.bundles[name]
	if !exists {
		return fmt.Errorf("bundle %s not exists", name)
	}
	for version := range bm.bundles[name] {
		err = bm.deleteByNameVersion(name, version)
		if err != nil {
			return
		}
	}
	return
}

func (bm *BundleManager) deleteByNameVersion(name string, version string) (err error) {
	id, exists := bm.getBundleID(name, version)
	if !exists {
		return fmt.Errorf("bundle %s (%s) not exists", name, version)
	}

	workDir := filepath.Join(bm.bundleDir, id)
	err = bm.umountOverlay(workDir)
	if err != nil {
		return fmt.Errorf("unable to umount %s (%s): [%v]", name, version, err)
	}
	err = os.RemoveAll(workDir)
	if err != nil {
		return fmt.Errorf("unable to remove dir %s: [%v]", workDir, err)
	}

	delete(bm.bundles[name], version)
	if len(bm.bundles[name]) == 0 {
		delete(bm.bundles, name)
	}
	return
}
