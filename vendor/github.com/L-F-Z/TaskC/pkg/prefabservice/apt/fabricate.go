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

package apt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

func Fabricate(path string, dstDir string) (prefabPath string, blueprintPath string, err error) {
	dir := filepath.Dir(path)
	tmpDir, err := os.MkdirTemp(dir, "APTFabricate-")
	if err != nil {
		err = errors.New("failed to create temp directory when unpacking deb: " + err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	rootDir := filepath.Join(tmpDir, "root")
	packing.Unpack(path, tmpDir, ".ar", "")
	if utils.PathExists(filepath.Join(tmpDir, "data.tar.xz")) {
		packing.Unpack(filepath.Join(tmpDir, "data.tar.xz"), rootDir, "", "")
	} else if utils.PathExists(filepath.Join(tmpDir, "data.tar.zst")) {
		packing.Unpack(filepath.Join(tmpDir, "data.tar.zst"), rootDir, "", "")
	} else if utils.PathExists(filepath.Join(tmpDir, "data.tar.gz")) {
		packing.Unpack(filepath.Join(tmpDir, "data.tar.gz"), rootDir, "", "")
	} else {
		err = errors.New("Cannot Find data.tar.* in " + path)
		return
	}

	metadataDir := filepath.Join(tmpDir, "metadata")
	if utils.PathExists(filepath.Join(tmpDir, "control.tar.xz")) {
		packing.Unpack(filepath.Join(tmpDir, "control.tar.xz"), metadataDir, "", "")
	} else if utils.PathExists(filepath.Join(tmpDir, "control.tar.zst")) {
		packing.Unpack(filepath.Join(tmpDir, "control.tar.zst"), metadataDir, "", "")
	} else if utils.PathExists(filepath.Join(tmpDir, "control.tar.gz")) {
		packing.Unpack(filepath.Join(tmpDir, "control.tar.gz"), metadataDir, "", "")
	} else {
		err = errors.New("Cannot Find control.tar.* in " + path)
		return
	}

	controlPath := filepath.Join(metadataDir, "control")
	if !utils.PathExists(controlPath) {
		err = errors.New("unable to find control file in deb file")
		return
	}
	info, err := loadControl(controlPath)
	if err != nil {
		err = fmt.Errorf("unable to load control file: %v", err)
		return
	}

	postinstPath := filepath.Join(metadataDir, "postinst")
	if utils.PathExists(postinstPath) {
		err = postInstall(postinstPath, rootDir)
		if err != nil {
			err = fmt.Errorf("unable to execute post install process: %v", err)
			return
		}
	}

	if info.name == "ca-certificates" {
		err = updateCaCertificates(rootDir)
		if err != nil {
			err = fmt.Errorf("error occured when update-ca-certificated for ca-certificate package: [%v]", err)
			return
		}
	}

	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_APT
	blueprint.Name = info.name
	blueprint.Version = info.version
	blueprint.Environment = info.architecture
	blueprint.Depend = append(info.preDepends, info.depends...)

	rePython := regexp.MustCompile(`python(\d+)\.(\d+)-minimal`)
	match := rePython.FindStringSubmatch(info.name)
	if len(match) == 3 {
		version := match[1] + "." + match[2]
		blueprint.Context.Set(dcontext.PYTHON_VERSION_KEY, version)
		blueprint.Context.Set(dcontext.PYTHON_BIN_PATH, "/usr/bin/python"+version)
	}
	return prefab.Pack(rootDir, dstDir, blueprint)
}

func FabricateVirtual(name string, dstDir string) (prefabPath string, blueprintPath string, err error) {
	provide, err := provideVirtualPackage(name)
	if err != nil {
		return
	}
	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_APT
	blueprint.Name = name
	blueprint.Version = VIRTUAL_PKG_VERSION
	blueprint.Environment = VIRTUAL_PKG_ENVIRONMENT
	blueprint.Depend = [][]*prefab.Prefab{provide}
	emptyDir, err := os.MkdirTemp("", "")
	if err != nil {
		return
	}
	defer os.RemoveAll(emptyDir)
	return prefab.Pack(emptyDir, dstDir, blueprint)
}
