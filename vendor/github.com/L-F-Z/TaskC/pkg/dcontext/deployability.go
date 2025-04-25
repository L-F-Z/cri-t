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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type Deployability map[string]string
type DeployabilityEvaluator func(string, *DeployContext) (int, error)

var DeployabilityEvaluators = make(map[string]DeployabilityEvaluator)

func (d *Deployability) String() string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(d)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	jsonStr := strings.TrimSpace(buf.String())
	return jsonStr[1 : len(jsonStr)-1]
}

func (d *Deployability) Add(key string, value string) {
	if *d == nil {
		*d = make(Deployability)
	}
	(*d)[key] = value
}

func ParseDeployability(jsonStr string) (*Deployability, error) {
	var d Deployability
	err := json.Unmarshal([]byte("{"+jsonStr+"}"), &d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// The evaluated deployability is an integer between 0 and 255
// 0 indicates not deployable
// 127 is the default result
// 255 indicates perfect match
func (dc *DeployContext) Evaluate(deployability *Deployability) (result int, err error) {
	if deployability == nil || len(*deployability) == 0 {
		return 127, nil
	}
	for key, specifier := range *deployability {
		evaluator, exists := DeployabilityEvaluators[key]
		if !exists {
			err = fmt.Errorf("deployability evaluator %s not found", key)
			return
		}
		r, eerr := evaluator(specifier, dc)
		if eerr != nil {
			err = fmt.Errorf("unable to evaluate %s: [%v]", key, eerr)
			return
		}
		result += r
	}
	result /= len(*deployability)
	return
}
