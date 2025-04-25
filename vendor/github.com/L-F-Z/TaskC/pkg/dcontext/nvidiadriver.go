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
	"strconv"
	"strings"
)

// for a full CUDA Toolkit and corresponding driver versions mapping, please refer to
// https://docs.nvidia.com/cuda/pdf/CUDA_Toolkit_Release_Notes.pdf

const NVIDIA_DRIVER_VERSION = "nvidia.driverVersion"
const CUDA_TOOLKIT_VERSION = "nvidia.cudaVersion"

func init() {
	DeployabilityEvaluators[CUDA_TOOLKIT_VERSION] = CUDAToolkitVersionEvaluator
}

func (d *DeployContext) SetNvidiaDriverVersion(root string) (err error) {
	nvidiaDriverVersion, err := NvidiaDriverVersion(root)
	if err != nil {
		return fmt.Errorf("unable to get nvidia driver version information: [%v]", err)
	}
	err = d.Set(NVIDIA_DRIVER_VERSION, nvidiaDriverVersion)
	if err != nil {
		return fmt.Errorf("unable to set nvidia driver version context: [%v]", err)
	}
	return
}

func NvidiaDriverVersion(root string) (version string, err error) {
	versionFile := filepath.Join(root, "/sys/module/nvidia/version")
	versionBytes, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}
	version = strings.TrimSpace(string(versionBytes))
	return version, nil
}

type NvidiaMap struct {
	minDriverReq []int
	CUDAVersion  int
}

var versionMapping = []NvidiaMap{
	{minDriverReq: []int{560, 28, 3}, CUDAVersion: 126},
	{minDriverReq: []int{555, 42, 2}, CUDAVersion: 125},
	{minDriverReq: []int{550, 54, 14}, CUDAVersion: 124},
	{minDriverReq: []int{545, 23, 6}, CUDAVersion: 123},
	{minDriverReq: []int{535, 54, 3}, CUDAVersion: 122},
	{minDriverReq: []int{530, 30, 2}, CUDAVersion: 121},
	{minDriverReq: []int{525, 60, 13}, CUDAVersion: 120},
	{minDriverReq: []int{520, 61, 5}, CUDAVersion: 118},
	{minDriverReq: []int{515, 43, 4}, CUDAVersion: 117},
	{minDriverReq: []int{510, 39, 1}, CUDAVersion: 116},
	{minDriverReq: []int{495, 29, 5}, CUDAVersion: 115},
	{minDriverReq: []int{470, 42, 1}, CUDAVersion: 114},
	{minDriverReq: []int{465, 19, 1}, CUDAVersion: 113},
	{minDriverReq: []int{460, 27, 3}, CUDAVersion: 112},
	{minDriverReq: []int{455, 23, 0}, CUDAVersion: 111},
	{minDriverReq: []int{450, 36, 6}, CUDAVersion: 110},
	{minDriverReq: []int{440, 33, 0}, CUDAVersion: 102},
	{minDriverReq: []int{418, 39, 0}, CUDAVersion: 101},
	{minDriverReq: []int{410, 48, 0}, CUDAVersion: 100},
	{minDriverReq: []int{396, 26, 0}, CUDAVersion: 92},
	{minDriverReq: []int{390, 46, 0}, CUDAVersion: 91},
	{minDriverReq: []int{384, 81, 0}, CUDAVersion: 90},
	{minDriverReq: []int{367, 48, 0}, CUDAVersion: 80},
	{minDriverReq: []int{352, 31, 0}, CUDAVersion: 75},
	{minDriverReq: []int{346, 46, 0}, CUDAVersion: 70},
}

func compareVersion(v1, v2 []int) int {
	for i := range len(v1) {
		if v1[i] > v2[i] {
			return 1
		} else if v1[i] < v2[i] {
			return -1
		}
	}
	return 0
}

// specifer is the CUDA toolkit Version, for example 12.6 or 11.5
// value is the nvidia driver version
func CUDAToolkitVersionEvaluator(specifier string, dc *DeployContext) (int, error) {
	value, exists := dc.Get(NVIDIA_DRIVER_VERSION)
	if !exists {
		return 0, nil // no GPU
	}
	driverVersion, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("received nvidia driver context value is not a string")
	}
	parts := strings.Split(driverVersion, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	v0, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("unable to decode NVIDIA driver version %s: [%v]", driverVersion, err)
	}
	v1, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("unable to decode NVIDIA driver version %s: [%v]", driverVersion, err)
	}
	v2, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("unable to decode NVIDIA driver version %s: [%v]", driverVersion, err)
	}

	latestSupport := 0
	for _, req := range versionMapping {
		cmp := compareVersion([]int{v0, v1, v2}, req.minDriverReq)
		if cmp >= 0 {
			latestSupport = req.CUDAVersion
			break
		}
	}

	parts = strings.Split(specifier, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("unable to decode CUDA version specifier %s", specifier)
	}
	s0, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("unable to decode CUDA version specifier %s: [%v]", specifier, err)
	}
	s1, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("unable to decode CUDA version specifier %s: [%v]", specifier, err)
	}
	current := 10*s0 + s1

	if current > latestSupport {
		return 0, nil
	}

	maxGap := versionMapping[0].CUDAVersion - versionMapping[len(versionMapping)-1].CUDAVersion
	gap := versionMapping[0].CUDAVersion - current
	return 256 - (gap * 128 / maxGap), nil
}
