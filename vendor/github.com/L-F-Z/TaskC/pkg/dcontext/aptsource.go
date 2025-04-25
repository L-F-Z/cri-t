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

// This tool is currently not used

package dcontext

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
)

// Ref: https://wiki.debian.org/SourcesList
func GetAptSources(root string) (srcs []string, err error) {
	sourcesListPath := filepath.Join(root, "/etc/apt/sources.list")
	repos, err := parseAptList(sourcesListPath)
	if err == nil {
		srcs = append(srcs, repos...)
	}

	sourcesListDPath := filepath.Join(root, "/etc/apt/sources.list.d")
	files, err := os.ReadDir(sourcesListDPath)
	if err != nil {
		return srcs, nil
	}
	for _, f := range files {
		sourceFilePath := filepath.Join(sourcesListDPath, f.Name())
		if strings.HasSuffix(f.Name(), ".list") {
			repos, err = parseAptList(sourceFilePath)
		} else if strings.HasSuffix(f.Name(), ".sources") {
			repos, err = parseAptSources(sourceFilePath)
		}
		if err == nil {
			srcs = append(srcs, repos...)
		}
	}
	return
}

func parseAptList(filePath string) (repos []string, err error) {
	if !utils.PathExists(filePath) {
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// delete optional part
		start := strings.Index(line, "[")
		end := strings.Index(line, "]")
		if start != -1 && end != -1 && end > start {
			line = line[:start] + line[end+1:]
		}

		// parse
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		if parts[0] != "deb" {
			continue
		}
		baseURL := parts[1]
		distribution := parts[2]
		components := parts[3:]
		for _, component := range components {
			fullURL := utils.CombineURL(baseURL, "dists", distribution, component)
			if fullURL != "" {
				repos = append(repos, fullURL)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}

func parseAptSources(filePath string) (repos []string, err error) {
	if !utils.PathExists(filePath) {
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var uris, suites, components []string
		for scanner.Text() != "" {
			line := scanner.Text()
			scanner.Scan()
			if line[0] == '#' {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "Types" && value != "deb" {
				// ignore "deb-src"
				break
			}
			switch key {
			case "URIs":
				uris = strings.Split(value, " ")
			case "Suites":
				suites = strings.Split(value, " ")
			case "Components":
				components = strings.Split(value, " ")
			}
		}
		for _, uri := range uris {
			for _, suite := range suites {
				for _, component := range components {
					fullURL := utils.CombineURL(uri, "dists", suite, component)
					if fullURL != "" {
						repos = append(repos, fullURL)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}
