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

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefab"
)

func (bm *BundleManager) Pack(blueprint prefab.Blueprint, basePath string, dstDir string, keepURL bool) (taskcPath string, blueprintPath string, err error) {
	saveDir, err := os.MkdirTemp("", "Bundle-Pack-")
	if err != nil {
		err = fmt.Errorf("unable to create build directory: [%v]", err)
		return
	}
	defer os.RemoveAll(saveDir)

	localCnt := 0
	for _, alternatives := range blueprint.Depend {
		for _, p := range alternatives {
			if p.SpecType != "LOCAL" && p.SpecType != "PYTHON" {
				continue
			}
			if p.SpecType == "LOCAL" && utils.IsURL(p.Specifier) && keepURL {
				continue
			}

			src := p.Specifier
			p.Specifier = intToShortName(localCnt)
			localCnt++
			dst := filepath.Join(saveDir, p.Specifier)
			err = os.MkdirAll(dst, 0700)
			if err != nil {
				err = fmt.Errorf("unable to create LOCAL prefab directory: [%v]", err)
				return
			}

			if utils.IsURL(src) {
				_, err = utils.Download(src, dst, "")
				if err != nil {
					err = fmt.Errorf("download %s failed: [%v]", src, err)
					return
				}
				continue
			}
			if !filepath.IsAbs(src) {
				src = filepath.Join(basePath, src)
			}
			if p.SpecType == "PYTHON" && utils.IsDir(src) {
				// preserve the name for python project directory
				dirName := filepath.Base(src)
				dst = filepath.Join(dst, dirName)
				p.Specifier = filepath.Join(p.Specifier, dirName)
			}
			err = utils.Copy(src, dst, true)
			if err != nil {
				err = fmt.Errorf("unable to copy LOCAL content to LOCAL prefab directory: [%v]", err)
				return
			}
		}
	}
	_, err = blueprint.Save(saveDir)
	if err != nil {
		err = fmt.Errorf("unable to write blueprint: [%v]", err)
		return
	}
	blueprintPath, err = blueprint.Save(dstDir)
	if err != nil {
		err = fmt.Errorf("unable to write blueprint: [%v]", err)
		return
	}

	filename := utils.SafeFilename(dstDir, ".taskc", blueprint.Name, blueprint.Version)
	err = packing.Pack(saveDir, dstDir, filename)
	if err != nil {
		err = fmt.Errorf("error occured when saving bundle: [%v]", err)
		return
	}
	taskcPath = filepath.Join(dstDir, filename)
	return
}
