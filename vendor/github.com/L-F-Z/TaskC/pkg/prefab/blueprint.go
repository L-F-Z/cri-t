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

package prefab

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
)

var getTag = regexp.MustCompile(`^\[\s*([^]]*)\s*\]\s*(.*)$`)

const (
	TAG_BLUEPRINT   = "BLUEPRINT"
	TAG_TYPE        = "TYPE"
	TAG_NAME        = "NAME"
	TAG_VERSION     = "VERSION"
	TAG_ENVIRONMENT = "ENVIRONMENT"
	TAG_USER        = "USER"
	TAG_WORKDIR     = "WORKDIR"
	TAG_TARGERDIR   = "TARGETDIR"
	TAG_ENVVAR      = "ENVVAR"
	TAG_ENTRYPOINT  = "ENTRYPOINT"
	TAG_CMD         = "CMD"
	TAG_DEPEND      = "DEPEND"
	TAG_CONTEXT     = "CONTEXT"
)

type Blueprint struct {
	Protocol    int
	Type        string
	Name        string
	Version     string
	Environment string
	// the relative path where the file should be saved with respect to the root directory.
	// When set to "" or "/", the file will be placed directly in the root directory.
	TargetDir  string
	User       string
	WorkDir    string
	EnvVar     []string // e.g. "PATH=/usr/local/bin:/usr/local/sbin"
	EntryPoint []string
	Command    []string // e.g. "python eaxmple.py"
	Depend     [][]*Prefab
	Context    *dcontext.DeployContext
}

func NewBlueprint() Blueprint {
	return Blueprint{
		Protocol: 1,
		Context:  new(dcontext.DeployContext),
	}
}

func (bp Blueprint) encode() (s string, err error) {
	if bp.Protocol <= 0 {
		return "", fmt.Errorf("invalid blueprint protocol number: %d", bp.Protocol)
	}
	s += "[" + TAG_BLUEPRINT + "] v" + strconv.Itoa(bp.Protocol) + "\n"

	if bp.Type == "" {
		return "", errors.New("empty blueprint type")
	}
	s += "[" + TAG_TYPE + "] " + bp.Type + "\n"

	if bp.Name == "" {
		return "", errors.New("empty blueprint name")
	}
	s += "[" + TAG_NAME + "] " + bp.Name + "\n"

	if bp.Version == "" {
		return "", errors.New("empty blueprint version")
	}
	s += "[" + TAG_VERSION + "] " + bp.Version + "\n"

	if bp.Environment == "" {
		s += "[" + TAG_ENVIRONMENT + "] any\n"
	} else {
		s += "[" + TAG_ENVIRONMENT + "] " + bp.Environment + "\n"
	}

	if bp.User != "" {
		s += "[" + TAG_USER + "] " + bp.User + "\n"
	}

	if bp.WorkDir != "" {
		s += "[" + TAG_WORKDIR + "] " + bp.WorkDir + "\n"
	}

	if bp.TargetDir != "" {
		s += "[" + TAG_TARGERDIR + "] " + bp.TargetDir + "\n"
	}

	if len(bp.EnvVar) > 0 {
		s += "[" + TAG_ENVVAR + "]\n"
		for _, env := range bp.EnvVar {
			s += "- " + env + "\n"
		}
	}

	if len(bp.EntryPoint) > 0 {
		s += "[" + TAG_ENTRYPOINT + "]\n"
		for _, ep := range bp.EntryPoint {
			s += "- " + ep + "\n"
		}
	}

	if len(bp.Command) > 0 {
		s += "[" + TAG_CMD + "]\n"
		for _, cmd := range bp.Command {
			s += "- " + cmd + "\n"
		}
	}

	if len(bp.Depend) > 0 {
		s += "[" + TAG_DEPEND + "]\n"
		for _, prefabs := range bp.Depend {
			for i, p := range prefabs {
				if i == 0 {
					s += "- "
				} else {
					s += "| "
				}
				s += p.String() + "\n"
			}
		}
	}

	if bp.Context != nil && len(*bp.Context) != 0 {
		s += "[" + TAG_CONTEXT + "] " + bp.Context.String() + "\n"
	}
	return
}

func (b *Blueprint) Save(path string) (blueprintPath string, err error) {
	encoded, err := b.encode()
	if err != nil {
		err = fmt.Errorf("unable to encode blueprint: %v", err)
		return
	}

	err = os.MkdirAll(path, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("unable to create the directory to store blueprint file: %v", err)
		return
	}

	blueprintName := utils.SafeFilename(path, ".blueprint", b.Name, b.Version, b.Environment)
	blueprintPath = filepath.Join(path, blueprintName)
	err = os.WriteFile(blueprintPath, []byte(encoded), os.ModePerm)
	if err != nil {
		err = fmt.Errorf("unable to save blueprint file: %v", err)
		return
	}
	return
}

func DecodeBlueprintFile(path string) (bp Blueprint, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("unable to open blueprint file at %s: [%v]", path, err.Error())
		return
	}
	return DecodeBlueprint(string(data))
}

func DecodeBlueprint(input string) (bp Blueprint, err error) {
	var currentTag string
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if line[0] == '/' && line[1] == '/' {
			continue
		}
		if (line[0] == '-' || line[0] == '|') && line[1] == ' ' {
			trimmed := strings.TrimSpace(line[2:])
			if len(trimmed) == 0 {
				fmt.Println("Warning: empty subitem line in blueprint")
				continue
			}
			switch currentTag {
			case TAG_ENVVAR:
				bp.EnvVar = append(bp.EnvVar, trimmed)
			case TAG_ENTRYPOINT:
				bp.EntryPoint = append(bp.EntryPoint, trimmed)
			case TAG_CMD:
				bp.Command = append(bp.Command, trimmed)
			case TAG_DEPEND:
				var p *Prefab
				p, err = parsePrefab(trimmed)
				if err != nil {
					err = errors.New("unable to parse prefab dependency line: " + trimmed)
					return
				}
				if line[0] == '-' {
					bp.Depend = append(bp.Depend, []*Prefab{p})
				} else { // line[0] == '|'
					bp.Depend[len(bp.Depend)-1] = append(bp.Depend[len(bp.Depend)-1], p)
				}
			default:
				err = errors.New("unknown Tag Group for subitem line: " + trimmed)
				return
			}
			continue
		}
		tagMatch := getTag.FindStringSubmatch(line)
		if len(tagMatch) != 3 {
			return bp, fmt.Errorf("no tag found for line: %s", line)
		}
		currentTag = tagMatch[1]
		content := tagMatch[2]
		switch currentTag {
		case TAG_BLUEPRINT:
			bp.Protocol, err = strconv.Atoi(content[1:])
			if err != nil {
				return bp, errors.New("cannot decode protocol version: " + line)
			}
		case TAG_TYPE:
			bp.Type = content
		case TAG_NAME:
			bp.Name = content
		case TAG_VERSION:
			bp.Version = content
		case TAG_ENVIRONMENT:
			bp.Environment = content
		case TAG_USER:
			bp.User = content
		case TAG_WORKDIR:
			bp.WorkDir = content
		case TAG_TARGERDIR:
			bp.TargetDir = content
		case TAG_CONTEXT:
			ctx, err := dcontext.ParseDeployContext(content)
			if err != nil {
				return bp, errors.New("cannot decode deploy context: " + line)
			}
			bp.Context = ctx
		case TAG_ENVVAR, TAG_ENTRYPOINT, TAG_CMD, TAG_DEPEND:
			continue
		default:
			err = errors.New("unknown Tag " + currentTag)
			return
		}
	}
	if scanner.Err() != nil {
		err = fmt.Errorf("error occured while decoding blueprint %v", scanner.Err())
	}
	return
}

func (b *Blueprint) AddDepend(p *Prefab) {
	b.Depend = append(b.Depend, []*Prefab{p})
}

func (b *Blueprint) AddDependAlternatives(p []*Prefab) {
	b.Depend = append(b.Depend, p)
}
