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

package dcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const AMD_ROCM_VERSION = "amd.rocmVersion"

func init() {
	DeployabilityEvaluators[AMD_ROCM_VERSION] = AMDROCmVersionEvaluator
}

func (d *DeployContext) SetAMDROCmVersion(root string) (err error) {
	AMDROCmVersion, err := AMDROCmVersion(root)
	if err != nil {
		return fmt.Errorf("unable to get AMD ROCm version: [%v]", err)
	}
	err = d.Set(AMD_ROCM_VERSION, AMDROCmVersion)
	if err != nil {
		return fmt.Errorf("unable to set AMD ROCm version context: [%v]", err)
	}
	return
}

func AMDROCmVersion(root string) (version string, err error) {
	optPath := filepath.Join(root, "opt")
	entries, err := os.ReadDir(optPath)
	if err != nil {
		return
	}
	regex := regexp.MustCompile(`^rocm-(\d+\.\d+)\.\d+$`)
	for _, entry := range entries {
		if entry.IsDir() {
			matches := regex.FindStringSubmatch(entry.Name())
			if len(matches) == 2 {
				return matches[1], nil
			}
		}
	}
	return
}

func AMDROCmVersionEvaluator(specifier string, dc *DeployContext) (int, error) {
	value, exists := dc.Get(AMD_ROCM_VERSION)
	if !exists {
		return 0, nil // no AMD GPU
	}
	ROCmVersion, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("received nvidia driver context value is not a string")
	}
	if specifier == ROCmVersion {
		return 255, nil
	} else {
		return 0, nil
	}
}
