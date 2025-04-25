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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func postInstall(postinstPath string, rootDir string) (err error) {
	file, err := os.Open(postinstPath)
	if err != nil {
		err = fmt.Errorf("error opening file: [%v]", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var command string

	re := regexp.MustCompile(`update-alternatives\s+--install\s+([^\s]+)\s+[^\s]+\s+([^\s]+)\s+\d+`)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "\\") {
			command += line[:len(line)-1] + " "
			continue
		}
		command += line
		if command != "" {
			matches := re.FindStringSubmatch(command)
			if len(matches) >= 3 {
				oldname := filepath.Join(rootDir, matches[2])
				newname := filepath.Join(rootDir, matches[1])
				if err := os.RemoveAll(newname); err != nil {
					return fmt.Errorf("failed to remove existing file or directory: [%v]", err)
				}
				if err := os.MkdirAll(filepath.Dir(newname), 0700); err != nil {
					return fmt.Errorf("failed to create directories: [%v]", err)
				}
				relativePath, err := filepath.Rel(filepath.Dir(newname), oldname)
				if err != nil {
					return fmt.Errorf("failed to calculate relative path: [%v]", err)
				}
				if err := os.Symlink(relativePath, newname); err != nil {
					return fmt.Errorf("failed to create symlink: [%v]", err)
				}
			}
			command = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: [%v]", err)
	}
	return
}
