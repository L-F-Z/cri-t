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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const LIBC_VERSION = "os.libcVersion"

func (d *DeployContext) SetLibCVersion(root string) (err error) {
	var major, minor int
	major, minor, err = LibCVersion(root)
	if err != nil {
		return fmt.Errorf("unable to get libc information: [%v]", err)
	}
	err = d.Set(LIBC_VERSION, fmt.Sprintf("%d.%d", major, minor))
	if err != nil {
		return fmt.Errorf("unable to set libc context: [%v]", err)
	}
	return
}

// Get system glibc version [major.minor]
// by reading "/var/lib/dpkg/status" and get package info of libc6
func LibCVersion(root string) (major int, minor int, err error) {
	pkgTarget := "libc6"
	path := filepath.Join(root, "/var/lib/dpkg/status")
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	var version string
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
			} else if key == "Version" {
				version = text[i+2:]
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	libc_regexp := regexp.MustCompile(`([0-9]+)\.([0-9]+)`)
	match := libc_regexp.FindStringSubmatch(version)
	if match == nil {
		err = errors.New(version + " not valid libc version")
		return
	}
	major, _ = strconv.Atoi(match[1])
	minor, _ = strconv.Atoi(match[2])
	return
}
