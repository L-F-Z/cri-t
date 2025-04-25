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

func GetAptInstalled(root string) (installed map[string]string, err error) {
	list := filepath.Join(root, "/var/lib/dpkg/status")
	file, err := os.Open(list)
	if err != nil {
		err = fmt.Errorf("failed to open dpkg status file: %v", err)
		return
	}
	defer file.Close()

	installed = make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var name, version string
		for scanner.Text() != "" {
			text := scanner.Text()
			scanner.Scan()
			if text[0] == ' ' {
				// a line start with space is part of a text block, which we don't care
				continue
			}
			i := 0
			for text[i] != ':' {
				i++
			}
			key := text[:i]
			if key == "Package" {
				name = text[i+2:]
			} else if key == "Version" {
				version = text[i+2:]
			}
		}
		installed[name] = version
	}
	if scanner.Err() != nil {
		err = fmt.Errorf("failed to read dpkg status file: %v", err)
	}
	return
}
