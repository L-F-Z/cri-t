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
	"os"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type packageInfo struct {
	name          string
	version       string
	architecture  string
	preDepends    [][]*prefab.Prefab
	depends       [][]*prefab.Prefab
	recommends    [][]*prefab.Prefab
	installedSize int
}

func _parseNameVersion(str string) [][]*prefab.Prefab {
	items := strings.Split(str, ", ")
	var parsed [][]*prefab.Prefab
	for _, str := range items {
		alternatives := strings.Split(str, " | ")
		var choices []*prefab.Prefab
		for _, item := range alternatives {
			var name, specifier string
			if item[len(item)-1] == ')' {
				pair := strings.Split(item, " (")
				name = strings.Split(pair[0], ":")[0]
				specifier = pair[1][:len(pair[1])-1]
			} else {
				name = strings.Split(item, ":")[0]
				specifier = "any"
			}
			if name == "libc-dev" {
				// this package doesn't exist
				continue
			}
			choices = append(choices, &prefab.Prefab{
				SpecType:  repointerface.REPO_APT,
				Name:      name,
				Specifier: specifier,
			})
		}
		parsed = append(parsed, choices)
	}
	return parsed
}

func loadControl(controlPath string) (p packageInfo, err error) {
	file, err := os.Open(controlPath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
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
			switch key {
			case "Package":
				p.name = text[i+2:]
			case "Version":
				p.version = text[i+2:]
			case "Architecture":
				p.architecture = text[i+2:]
			case "Pre-Depends":
				p.preDepends = _parseNameVersion(text[i+2:])
			case "Depends":
				p.depends = _parseNameVersion(text[i+2:])
			case "Recommends":
				p.recommends = _parseNameVersion(text[i+2:])
			case "Installed-Size":
				p.installedSize, _ = strconv.Atoi(text[i+2:])
			}
		}
	}
	err = scanner.Err()
	return
}
