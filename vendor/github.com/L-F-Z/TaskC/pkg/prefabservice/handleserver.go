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

package prefabservice

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"path/filepath"

	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

func (p *PrefabService) HandleList(writer io.Writer) (err error) {
	tmpl, err := template.New("info").Parse(tpl)
	if err != nil {
		return err
	}
	p.infoStore.RLock()
	defer p.infoStore.RUnlock()
	return tmpl.Execute(writer, p.infoStore)
}

const tpl = `
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Prefab Server List</title>
<style>
details { margin-left: 20px; }
summary { cursor: pointer; }
ul { margin-top: 0px; margin-bottom: 0px; padding-left: 20px; }
li { margin-top: 0px; margin-bottom: 0px; margin-left: 20px; }
</style>
</head>
<body>
<h1>Prefab Server Information</h1>
{{- range $repoName, $repoInfo := .Repos }}
<details>
	<summary><strong>Repository:</strong> {{ $repoName }}</summary>
	{{- range $name, $nameInfo := $repoInfo.Names }}
	<details>
		<summary><strong>Name:</strong> {{ $name }}</summary>
		{{- range $version, $versionInfo := $nameInfo.Versions }}
		<details>
			<summary><strong>Version:</strong> {{ $version }}</summary>
			<ul>
				{{- range $env, $item := $versionInfo.Environments }}
				<li>
					<strong>Environment:</strong> {{ $env }} 
					<a href="javascript:void(0);" onclick="deleteItem('{{ $repoName }}', '{{ $name }}', '{{ $version }}', '{{ $env }}')">DELETE</a>
				</li>
				{{- end }}
			</ul>
		</details>
		{{- end }}
	</details>
	{{- end }}
</details>
{{- end }}

<script>
function deleteItem(repo, name, version, environment) {
const url = '/item?repo=' + encodeURIComponent(repo) + '&name=' + encodeURIComponent(name) + '&version=' + encodeURIComponent(version) + '&environment=' + encodeURIComponent(environment);
	fetch(url, {
		method: 'DELETE'
	})
	.then(response => {
		if (response.ok) {
			alert('Item deleted successfully');
			location.reload(); // Optional: Reload the page after deletion
		} else {
			alert('Failed to delete item');
		}
	})
	.catch(error => {
		alert('Error deleting item');
	});
}
</script>
</body>
</html>
`

func (ps *PrefabService) HandleGetNames(repository string) (names []string, err error) {
	if repository == "" {
		err = errors.New("incomplete query, Repository is empty")
		return
	}
	names, outdated := ps.infoStore.GetNames(repository)

	if !outdated {
		return
	}
	if ps.upstream == "" {
		return
	}
	upstreamNames, err := ps.GetUpstreamNames(repository)
	if err != nil {
		err = fmt.Errorf("failed to get upstream names: [%v]", err)
		return
	}
	ps.infoStore.SetNames(repository, upstreamNames)
	names, _ = ps.infoStore.GetNames(repository)
	return
}

func (ps *PrefabService) HandleGetVersions(repository string, name string) (versions []string, err error) {
	if repository == "" {
		err = errors.New("incomplete query, Repository is empty")
		return
	}
	if name == "" {
		err = errors.New("incomplete query, Name is empty")
		return
	}
	versions, outdated := ps.infoStore.GetVersions(repository, name)

	if !outdated {
		return
	}
	if ps.upstream == "" {
		return
	}
	upstreamVersions, err := ps.GetUpstreamVersions(repository, name)
	if err != nil {
		err = fmt.Errorf("failed to get upstream versions: [%v]", err)
		return
	}
	ps.infoStore.SetVersions(repository, name, upstreamVersions)
	versions, _ = ps.infoStore.GetVersions(repository, name)
	return
}

func (ps *PrefabService) HandleGetEnvironments(repository string, name string, version string) (environments []string, err error) {
	if repository == "" {
		err = errors.New("incomplete query, Repository is empty")
		return
	}
	if name == "" {
		err = errors.New("incomplete query, Name is empty")
		return
	}
	if version == "" {
		err = errors.New("incomplete query, Version is empty")
		return
	}
	environments, outdated := ps.infoStore.GetEnvironments(repository, name, version)

	if !outdated {
		return
	}
	if ps.upstream == "" {
		return
	}
	upstreamEnvs, err := ps.GetUpstreamEnvironemnts(repository, name, version)
	if err != nil {
		err = fmt.Errorf("failed to get upstream environments: [%v]", err)
		return
	}
	ps.infoStore.SetEnvironments(repository, name, version, upstreamEnvs)
	environments, _ = ps.infoStore.GetEnvironments(repository, name, version)
	return
}

func (ps *PrefabService) HandleGetItem(repository, name, version, environment string) (prefabID, blueprintID string, err error) {
	if repository == "" {
		err = errors.New("incomplete query, Repository is empty")
		return
	}
	if name == "" {
		err = errors.New("incomplete query, Name is empty")
		return
	}
	if version == "" {
		err = errors.New("incomplete query, Version is empty")
		return
	}
	if environment == "" {
		err = errors.New("incomplete query, Environment is empty")
		return
	}
	prefabID, blueprintID = ps.infoStore.GetItem(repository, name, version, environment)

	if prefabID != "" && blueprintID != "" {
		return
	}
	if ps.upstream == "" {
		return
	}
	upstreamPrefabID, upstreamBlueprintID, err := ps.GetUpstreamItem(repository, name, version, environment)
	if err != nil {
		err = fmt.Errorf("failed to get upstream item: [%v]", err)
		return
	}
	ps.infoStore.SetItem(repository, name, version, environment, upstreamPrefabID, upstreamBlueprintID)
	prefabID, blueprintID = ps.infoStore.GetItem(repository, name, version, environment)
	return
}

func (ps *PrefabService) HandleDeleteItem(repository string, name string, version string, environment string) (err error) {
	prefabID, blueprintID := ps.infoStore.GetItem(repository, name, version, environment)
	err = ps.infoStore.DeleteItem(repository, name, version, environment)
	if err != nil {
		return
	}
	if prefabID != "" {
		err = ps.fileStore.DeleteFile(prefabID)
		if err != nil {
			return
		}
	}
	if blueprintID != "" {
		err = ps.fileStore.DeleteFile(blueprintID)
		if err != nil {
			return
		}
	}
	return
}

func (ps *PrefabService) HandleGetFile(id string) (path string, fileName string, fileType string, err error) {
	return ps.provideFile(id)
}

func (ps *PrefabService) HandlePostSpecSheet(specSheet []byte) (prefabID string, blueprintID string, err error) {
	spec, err := DecodeSpecSheet(specSheet)
	if err != nil {
		return
	}
	log.Printf("Handling request [%s] %s [%v]\n", spec.Type, spec.Name, spec.Specifier)
	return ps.PrefabSelection(spec)
}

func (ps *PrefabService) HandlePostUpload(repository, prefabPath, blueprintPath, fileType string) (prefabID, blueprintID string, err error) {
	if repository == "" {
		err = errors.New("incomplete query, Repository is empty")
		return
	}
	blueprint, err := prefab.DecodeBlueprintFile(blueprintPath)
	if err != nil {
		err = fmt.Errorf("unable to decode blueprint file %s: [%v]", filepath.Base(blueprintPath), err)
		return
	}
	err = ps.HandleDeleteItem(repository, blueprint.Name, blueprint.Version, blueprint.Environment)
	if err != nil {
		err = fmt.Errorf("error occured when deleting existing files: [%v]", err)
		return
	}
	prefabID, err = ps.fileStore.NewFile(prefabPath, fileType)
	if err != nil {
		err = fmt.Errorf("error occured when uploading prefab file: [%v]", err)
		return
	}
	blueprintID, err = ps.fileStore.NewFile(blueprintPath, repointerface.FILETYPE_RAW)
	if err != nil {
		err = fmt.Errorf("error occured when uploading blueprint file: [%v]", err)
		return
	}
	err = ps.infoStore.SetItem(repository, blueprint.Name, blueprint.Version, blueprint.Environment, prefabID, blueprintID)
	if err != nil {
		err = fmt.Errorf("failed to add prefab/blueprint to info store: [%v]", err)
		return
	}
	return
}
