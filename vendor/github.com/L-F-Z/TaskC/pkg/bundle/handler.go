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
	"strings"

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type PackConfig struct {
	BundleName     string
	BundleVersion  string
	BlueprintPath  string // ignore the BlueprintPath when BundleName and BundleVersion are given
	NewName        string
	NewVersion     string
	DstDir         string
	KeepURL        bool
	UploadRegistry bool
}

func (bm *BundleManager) PackHandler(cfg PackConfig) (err error) {
	if cfg.DstDir == "" {
		cfg.DstDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: [%v]", err)
		}
	}
	cfg.DstDir, err = filepath.Abs(cfg.DstDir)
	if err != nil {
		return fmt.Errorf("failed to convert dstDir to absolute path: [%v]", err)
	}
	err = utils.EnsureDir(cfg.DstDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to check dstDir %s: [%v]", cfg.DstDir, err)
	}

	var blueprint prefab.Blueprint
	var basePath string
	if cfg.BundleName != "" && cfg.BundleVersion != "" {
		bundle, err := bm.Get(cfg.BundleName, cfg.BundleVersion)
		if err != nil {
			return fmt.Errorf("task bundle %s (%s) not found", cfg.BundleName, cfg.BundleVersion)
		}
		blueprint = *bundle.Blueprint
		basePath = bundle.BasePath
	} else {
		blueprintPath, err := filepath.Abs(cfg.BlueprintPath)
		if err != nil {
			return fmt.Errorf("failed to convert blueprint path to absolute path: [%v]", err)
		}
		blueprint, err = prefab.DecodeBlueprintFile(blueprintPath)
		if err != nil {
			return fmt.Errorf("failed to decode Blueprint file: [%v]", err)
		}
		basePath = filepath.Dir(blueprintPath)
	}
	if cfg.NewName != "" {
		blueprint.Name = cfg.NewName
	}
	if cfg.NewVersion != "" {
		blueprint.Version = cfg.NewVersion
	}
	taskcPath, blueprintPath, err := bm.Pack(blueprint, basePath, cfg.DstDir, cfg.KeepURL)
	if err != nil {
		return fmt.Errorf("failed to pack Task Closure: [%v]", err)
	}
	if cfg.UploadRegistry {
		err = bm.Upload(repointerface.REPO_CLOSURE, taskcPath, blueprintPath)
		if err != nil {
			return fmt.Errorf("failed to upload Task Closure: [%v]", err)
		}
	}
	return
}

type AssembleConfig struct {
	ClosureName         string
	ClosureVersion      string
	ClosurePath         string // ignore the ClosurePath when ClosureName and ClosureVersion are given
	NewName             string
	NewVersion          string
	Overwrite           bool
	IgnoreGPU           bool
	NvidiaDriverVersion string
}

func (bm *BundleManager) AssembleHandler(cfg AssembleConfig) error {
	tempDir, err := os.MkdirTemp("", "asm")
	if err != nil {
		return fmt.Errorf("failed to create a temp directory for assembling: [%v]", err)

	}
	defer os.RemoveAll(tempDir)

	var closurePath string
	if cfg.ClosureName != "" && cfg.ClosureVersion != "" {
		if bm.Exist(cfg.ClosureName, cfg.ClosureVersion) && !cfg.Overwrite {
			return nil
		}
		closureFilename, err := bm.RequestClosure(cfg.ClosureName, cfg.ClosureVersion, tempDir)
		if err != nil {
			return fmt.Errorf("failed to get Task Closure from registry: [%v]", err)
		}
		closurePath = filepath.Join(tempDir, closureFilename)
	} else if utils.IsURL(cfg.ClosurePath) {
		closureFilename, err := utils.Download(cfg.ClosurePath, tempDir, "")
		if err != nil {
			return fmt.Errorf("failed to download Task Closure from %v: [%v]", cfg.ClosurePath, err)
		}
		closurePath = filepath.Join(tempDir, closureFilename)
	} else {
		closurePath = cfg.ClosurePath
	}

	// Unpack the Task Closure
	err = packing.Unpack(closurePath, tempDir, ".tar.gz", "")
	if err != nil {
		return fmt.Errorf("failed to unpack Task Closure: [%v]", err)
	}

	// Load the blueprint in Task Closure
	blueprint, err := findBlueprint(tempDir)
	if err != nil {
		return fmt.Errorf("failed to load blueprint: [%v]", err)
	}

	if cfg.NewName != "" {
		blueprint.Name = cfg.NewName
	}
	if cfg.NewVersion != "" {
		blueprint.Version = cfg.NewVersion
	}
	if bm.Exist(blueprint.Name, blueprint.Version) {
		if cfg.Overwrite {
			err := bm.DeleteBundle(blueprint.Name, blueprint.Version)
			if err != nil {
				return fmt.Errorf("failed to remove existing bundle, failed to overwrite: [%v]", err)
			}
		} else {
			return fmt.Errorf("Bundle %s (%s) already exists, use --overwrite or delete it first", blueprint.Name, blueprint.Version)
		}
	}

	// set deployment context
	dctx := new(dcontext.DeployContext)
	err = dctx.SetArch("/")
	if err != nil {
		return fmt.Errorf("failed to get hardware architecture: [%v]", err)
	}
	if !cfg.IgnoreGPU {
		dctx.SetNvidiaDriverVersion("/")
		dctx.SetAMDROCmVersion("/")
	}
	if cfg.NvidiaDriverVersion != "" {
		dctx.Set(dcontext.NVIDIA_DRIVER_VERSION, cfg.NvidiaDriverVersion)
	}
	return bm.Assemble(blueprint, tempDir, dctx)
}

func findBlueprint(dirPath string) (blueprint prefab.Blueprint, err error) {
	var blueprintPath string
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		err = fmt.Errorf("failed to read directory %s: [%v]", dirPath, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".blueprint") {
			blueprintPath = filepath.Join(dirPath, entry.Name())
			break
		}
	}
	if blueprintPath == "" {
		err = fmt.Errorf("no blueprint file found in %s", dirPath)
		return
	}
	return prefab.DecodeBlueprintFile(blueprintPath)
}
