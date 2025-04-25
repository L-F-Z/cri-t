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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

const ARCH_KEY = "hardware.architecture"

func init() {
	DeployabilityEvaluators[ARCH_KEY] = ArchEvaluator
}

func (d *DeployContext) SetArch(root string) (err error) {
	arch, err := Arch(root)
	if err != nil {
		return fmt.Errorf("unable to get arch information: [%v]", err)
	}
	err = d.Set(ARCH_KEY, arch)
	if err != nil {
		return fmt.Errorf("unable to set arch context: [%v]", err)
	}
	return
}

// Get system architure by reading "/var/lib/dpkg/status" and get package info of libc6
// All possible archs includes: amd64, arm64, armel, armhf, i386, mips64el, mipsel, ppc64el, riscv64, s390x
func Arch(root string) (arch string, err error) {
	pkgTarget := "libc6"
	path := filepath.Join(root, "/var/lib/dpkg/status")
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	found := false
	for scanner.Scan() {
		if found {
			break
		}
		for scanner.Text() != "" {
			text := scanner.Text()
			scanner.Scan()
			if text[0] == ' ' {
				continue
			}
			i := 0
			for text[i] != ':' {
				i++
			}
			key := text[:i]
			if key == "Package" {
				if text[i+2:] != pkgTarget {
					break
				} else {
					found = true
				}
			} else if key == "Architecture" {
				arch = text[i+2:]
			}
		}
	}
	err = scanner.Err()
	return
}

func ArchEvaluator(specifier string, dc *DeployContext) (int, error) {
	value, exists := dc.Get(ARCH_KEY)
	if !exists {
		return 0, fmt.Errorf("key %s not found in deployment context", ARCH_KEY)
	}
	localArch, ok := value.(string)
	if !ok {
		return 0, fmt.Errorf("received arch context value is not a string")
	}
	switch localArch {
	case "amd64":
		if specifier == "x86_64" {
			return 255, nil
		}
	case "i386":
		if specifier == "i686" || specifier == "i386" {
			return 255, nil
		}
	case "arm64":
		if specifier == "aarch64" {
			return 255, nil
		}
	}
	if specifier == localArch {
		return 255, nil
	}
	return 0, nil
}
