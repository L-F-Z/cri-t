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

package libtc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func addResourceLimit(spec *specs.Spec, cpuLimit string, memoryLimit string) (err error) {
	if cpuLimit == "" && memoryLimit == "" {
		return
	}

	if spec.Linux == nil {
		spec.Linux = &specs.Linux{}
	}
	if spec.Linux.Resources == nil {
		spec.Linux.Resources = &specs.LinuxResources{}
	}

	if cpuLimit != "" {
		cpuFloat, err := strconv.ParseFloat(cpuLimit, 64)
		if err != nil {
			return fmt.Errorf("invalid CPU limit '%s': %v", cpuLimit, err)
		}

		cpuQuota := int64(cpuFloat * 100000)
		spec.Linux.Resources.CPU = &specs.LinuxCPU{
			Quota: &cpuQuota,
		}

		availableCPUs := max(int(cpuFloat), 1)
		spec.Process.Env = append(spec.Process.Env, fmt.Sprintf("OMP_NUM_THREADS=%d", availableCPUs))
		spec.Process.Env = append(spec.Process.Env, fmt.Sprintf("MKL_NUM_THREADS=%d", availableCPUs))
	}

	if memoryLimit != "" {
		memLimit, err := parseMemoryLimit(memoryLimit)
		if err != nil {
			return fmt.Errorf("invalid memory limit '%s': %v", memoryLimit, err)
		}
		spec.Linux.Resources.Memory = &specs.LinuxMemory{
			Limit: &memLimit,
		}
	}
	return
}

func parseMemoryLimit(mem string) (int64, error) {
	var multiplier int64 = 1
	if strings.HasSuffix(mem, "K") || strings.HasSuffix(mem, "k") {
		multiplier = 1024
		mem = strings.TrimSuffix(mem, "K")
		mem = strings.TrimSuffix(mem, "k")
	} else if strings.HasSuffix(mem, "M") || strings.HasSuffix(mem, "m") {
		multiplier = 1024 * 1024
		mem = strings.TrimSuffix(mem, "M")
		mem = strings.TrimSuffix(mem, "m")
	} else if strings.HasSuffix(mem, "G") || strings.HasSuffix(mem, "g") {
		multiplier = 1024 * 1024 * 1024
		mem = strings.TrimSuffix(mem, "G")
		mem = strings.TrimSuffix(mem, "g")
	}

	memInt, err := strconv.ParseInt(mem, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %v", err)
	}

	return memInt * multiplier, nil
}
