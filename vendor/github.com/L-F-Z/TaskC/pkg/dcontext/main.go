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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

type DeployContext map[string]any

func (d DeployContext) String() string {
	jsonData, err := json.Marshal(d)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(jsonData)
}

func ParseDeployContext(jsonStr string) (d *DeployContext, err error) {
	d = new(DeployContext)
	err = json.Unmarshal([]byte(jsonStr), d)
	return
}

func isValidType(x any) bool {
	// currently only support some JSON serializable types
	t := reflect.TypeOf(x)

	// Helper function to recursively check validity
	var checkType func(reflect.Type) bool
	checkType = func(t reflect.Type) bool {
		switch t.Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64, reflect.String, reflect.Interface:
			return true
		case reflect.Array, reflect.Slice:
			return checkType(t.Elem())
		case reflect.Map:
			return t.Key().Kind() == reflect.String && checkType(t.Elem())
		default:
			return false
		}
	}
	return checkType(t)
}

func (d DeployContext) Has(key string) bool {
	_, ok := d[key]
	return ok
}

// for a given key,
// if the key doesn't exist in the context, directly add the value to the context
// if the key exists in the context, switch the current value to the new value
func (d *DeployContext) Set(key string, value any) (err error) {
	if d == nil {
		return errors.New("DeployContext is nil; please initialize it before use")
	}
	if *d == nil {
		*d = make(DeployContext)
	}
	if !isValidType(value) {
		return errors.New("unsupported context type " + reflect.TypeOf(value).String())
	}
	(*d)[key] = value
	return
}

func (d *DeployContext) Get(key string) (data any, exists bool) {
	data, exists = (*d)[key]
	return
}

func (d *DeployContext) Merge(newContext *DeployContext) (err error) {
	if newContext == nil {
		return
	}
	for key, value := range *newContext {
		err = d.Set(key, value)
		if err != nil {
			return
		}
	}
	return
}

// Works for slice and array
func (d *DeployContext) SliceContains(key string, query any) (exist bool, err error) {
	value, ok := (*d)[key]
	if !ok {
		return false, fmt.Errorf("context not found for key [%s]", key)
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return false, fmt.Errorf("context [%s] is not a slice or array, but is type %s", key, v.Type())
	}
	if v.Type().Elem() != reflect.TypeOf(query) {
		return false, fmt.Errorf("context [%s] has %s type element, but the query is type %s, not matched", key, v.Type().Elem(), reflect.TypeOf(query))
	}
	for i := range v.Len() {
		elem := v.Index(i).Interface()
		if elem == query {
			return true, nil
		}
	}
	return false, nil
}

func (d *DeployContext) SliceAppend(key string, newItem any) error {
	value, ok := (*d)[key]
	if !ok {
		// If no existing entry, create a new slice with the newItem and add to the context
		newSlice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(newItem)), 0, 1)
		newSlice = reflect.Append(newSlice, reflect.ValueOf(newItem))
		return d.Set(key, newSlice)
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("context [%s] is not a slice, but is type %s", key, v.Type())
	}
	if v.Type().Elem() != reflect.TypeOf(newItem) {
		return fmt.Errorf("context [%s] has %s type element, but newItem is type %s, not matched", key, v.Type().Elem(), reflect.TypeOf(newItem))
	}
	newSlice := reflect.Append(v, reflect.ValueOf(newItem))
	return d.Set(key, newSlice.Interface())
}

func (d *DeployContext) MapSet(key string, mapKey string, mapValue any) error {
	if !isValidType(mapValue) {
		return fmt.Errorf("unsupported value type %s", reflect.TypeOf(mapValue))
	}
	value, ok := (*d)[key]
	if !ok {
		value = make(map[string]any)
	}
	v, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("context [%s] is not a map", key)
	}
	v[mapKey] = mapValue
	return d.Set(key, v)
}

func (d *DeployContext) MapGet(key string, mapKey string) (value any, err error) {
	value, ok := (*d)[key]
	if !ok {
		return nil, fmt.Errorf("context not found for key [%s]", key)
	}
	v, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("context [%s] is not a map", key)
	}
	value, exists := v[mapKey]
	if !exists {
		return nil, fmt.Errorf("key [%s] not found in context [%s]", mapKey, key)
	}
	return value, nil
}
