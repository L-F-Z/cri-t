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

package pypi

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

func selectPyVerAbis(candidates []pkgEnv, pyVer string) (selected []pkgEnv) {
	allows, err := pyVerAbisOrder(pyVer)
	if err != nil {
		return
	}
	for _, cand := range candidates {
		for _, pyVerAbi := range cand.pyVerAbis {
			if slices.Contains(allows, pyVerAbi) {
				selected = append(selected, cand)
			}
		}
	}
	return
}

func selectPlatform(candidates []pkgEnv, arch string, libcVer string) (best pkgEnv) {
	order, err := platformsOrder(arch, libcVer)
	if err != nil {
		return
	}
	min := len(order)

	for _, cand := range candidates {
		for _, plat := range cand.platforms {
			ord := slices.Index(order, plat)
			if ord == -1 || ord >= min {
				continue
			}
			min = ord
			best = cand
		}
	}
	return
}

func pyVerAbisOrder(pyVer string) (tags []string, err error) {
	// Currently we assume using cpython interpreter
	parts := strings.Split(pyVer, ".")
	if len(parts) > 2 {
		err = errors.New("unable to decode pyVer " + pyVer)
		return
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		err = errors.New("unable to decode pyVer " + pyVer)
		return
	}

	if len(parts) == 1 {
		tags = append(tags, "py"+strconv.Itoa(major)+"-none")
		return
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		err = errors.New("unable to decode pyVer " + pyVer)
		return
	}

	interpreter := "cp" + strconv.Itoa(major) + strconv.Itoa(minor)
	abi := interpreter
	if major <= 3 && minor <= 7 {
		abi += "m"
	}
	tags = append(tags, interpreter+"-"+abi)
	if major >= 3 && minor >= 2 {
		tags = append(tags, interpreter+"-abi3")
	}
	tags = append(tags, interpreter+"-none")
	tags = append(tags, "py"+strconv.Itoa(major)+strconv.Itoa(minor)+"-none")
	tags = append(tags, "py"+strconv.Itoa(major)+"-none")
	for i := minor - 1; i >= 0; i-- {
		tags = append(tags, "py"+strconv.Itoa(major)+strconv.Itoa(i)+"-none")
	}
	if major == 3 {
		for i := minor - 1; i >= 0; i-- {
			tags = append(tags, "cp"+strconv.Itoa(major)+strconv.Itoa(i)+"-abi3")
		}
	}
	return
}

func platformsOrder(arch string, libcVer string) (platforms []string, err error) {
	// decode libcVer
	parts := strings.Split(libcVer, ".")
	if len(parts) != 2 {
		err = errors.New("unable to decode libcVer " + libcVer)
		return
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		err = errors.New("unable to decode libcVer " + libcVer)
		return
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		err = errors.New("unable to decode libcVer " + libcVer)
		return
	}

	if arch == "amd64" {
		arch = "x86_64"
	}
	if arch == "i386" {
		arch = "i686"
	}
	if arch == "arm64" {
		arch = "aarch64"
	}

	too_old_minor := 16
	if arch == "x86_64" || arch == "i686" {
		too_old_minor = 4
	}

	// Currently we only support major == 2
	for m := minor; m > too_old_minor; m-- {
		platforms = append(platforms, "manylinux_"+strconv.Itoa(major)+"_"+strconv.Itoa(m)+"_"+arch)
		if m == 17 {
			platforms = append(platforms, "manylinux2014_"+arch)
		} else if m == 12 {
			platforms = append(platforms, "manylinux2010_"+arch)
		} else if m == 5 {
			platforms = append(platforms, "manylinux1_"+arch)
		}
	}
	platforms = append(platforms, "linux_"+arch)
	platforms = append(platforms, "any")
	return
}
