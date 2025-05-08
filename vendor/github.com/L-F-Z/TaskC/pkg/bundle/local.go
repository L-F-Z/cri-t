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
	"slices"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
)

const LOCAL_CONTENT_TAG = "LOCAL"
const LOCAL_PYTHON_TAG = "PYTHON"

var localTypes = []string{LOCAL_CONTENT_TAG, LOCAL_PYTHON_TAG}

func (bm *BundleManager) assembleLocal(bundle *Bundle, blueprint *prefab.Blueprint, ctx *dcontext.DeployContext) (err error) {
	for _, alternative := range blueprint.Depend {
		for _, cand := range alternative {
			if !slices.Contains(localTypes, cand.SpecType) {
				continue
			}
			if len(alternative) != 1 {
				return fmt.Errorf("[%s] %s [%s] must not have alternatives", cand.SpecType, cand.Name, cand.Specifier)
			}

			dstDir := filepath.Join(bundle.LocalDir, utils.IntToShortName(bundle.LocalDirCnt))
			bundle.LocalDirCnt++
			bundle.PrefabPaths = append(bundle.PrefabPaths, dstDir)

			switch cand.SpecType {
			case LOCAL_CONTENT_TAG:
				err = asmLocal(cand, bundle.BasePath, dstDir)
			case LOCAL_PYTHON_TAG:
				err = asmPython(cand, bundle.BasePath, dstDir, ctx)
			}
			if err != nil {
				return err
			}
		}
	}
	return
}

func asmLocal(p *prefab.Prefab, basePath string, dstDir string) (err error) {
	targetPath := filepath.Join(dstDir, p.Name)
	src := p.Specifier
	if utils.IsURL(src) {
		_, err := utils.Download(src, targetPath, "")
		if err != nil {
			return fmt.Errorf("download Local Prefab failed: [%v]", err)
		}
	} else {
		if !filepath.IsAbs(src) {
			src = filepath.Join(basePath, src)
		}
		err = utils.Copy(src, targetPath, true)
		if err != nil {
			return fmt.Errorf("unable to copy %s -> %s: [%v]", src, targetPath, err)
		}
	}
	p.Specifier = targetPath
	return
}

func asmPython(p *prefab.Prefab, basePath string, dstDir string, ctx *dcontext.DeployContext) (err error) {
	targetPath := filepath.Join(dstDir, "/usr/local/lib/python-site-packages")
	src := p.Specifier
	if !filepath.IsAbs(src) {
		src = filepath.Join(basePath, src)
	}
	pkgName := filepath.Base(src)
	targetPath = filepath.Join(targetPath, pkgName)
	p.Specifier = targetPath
	err = utils.Copy(src, targetPath, true)
	if err != nil {
		return fmt.Errorf("unable to copy %s -> %s: [%v]", src, targetPath, err)
	}

	// create Python package entrypoint
	if p.Name == "" {
		return
	}
	parts := strings.Split(p.Name, ":")
	if len(parts) != 3 {
		return fmt.Errorf("unable to parse PYTHON entrypoint info: %s", p.Name)
	}
	binName, pkg, function := parts[0], parts[1], parts[2]
	pythonBinPathCtx, exists := ctx.Get(dcontext.PYTHON_BIN_PATH)
	if !exists {
		return fmt.Errorf("unable to get python bin path from context: [%v]", err)
	}
	pythonBinPath, ok := pythonBinPathCtx.(string)
	if !ok {
		return fmt.Errorf("python bin path in context is not a string")
	}
	script := genPythonEntrypoint(pythonBinPath, pkg, function)
	targetPath = filepath.Join(dstDir, "/usr/local/bin", binName)
	err = os.MkdirAll(filepath.Dir(targetPath), 0700)
	if err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	err = os.WriteFile(targetPath, script, 0700)
	if err != nil {
		return fmt.Errorf("failed to write python entrypoint script: %v", err)
	}
	return
}

func genPythonEntrypoint(pythonBinPath string, pkg string, function string) []byte {
	scriptContent := `#!%s
# -*- coding: utf-8 -*-
import re
import sys
from %s import %s
if __name__ == '__main__':
    sys.argv[0] = re.sub(r'(-script\.pyw|\.exe)?$', '', sys.argv[0])
    sys.exit(%s())`
	script := fmt.Sprintf(scriptContent, pythonBinPath, pkg, function, function)
	return []byte(script)
}
