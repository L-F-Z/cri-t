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

package repointerface

import (
	"encoding/json"
)

type SpecSheet struct {
	Type      string
	Name      string
	Version   Version
	Env       string
	Specifier Constraint
	EnvSpec   EnvSpec
}

func (specSheet SpecSheet) Encode() (encoded []byte, err error) {
	s := &SpecSheetString{
		Type: specSheet.Type,
		Name: specSheet.Name,
		Env:  specSheet.Env,
	}
	if specSheet.Version != nil {
		s.Version = specSheet.Version.String()
	}
	if specSheet.EnvSpec != nil {
		s.EnvSpec = specSheet.EnvSpec.Encode()
	}
	s.Specifier, err = specSheet.Specifier.Encode()
	if err != nil {
		return
	}
	return s.Encode(), nil
}

type SpecSheetString struct {
	Type      string
	Name      string
	Version   string
	Env       string
	Specifier string
	EnvSpec   string
}

func (s *SpecSheetString) Encode() []byte {
	enc, _ := json.Marshal(s)
	return enc
}

func (s *SpecSheetString) Decode(raw []byte) error {
	return json.Unmarshal(raw, s)
}

type EnvSpec interface {
	Encode() string
}
