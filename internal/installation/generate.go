package installation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"path"
	"sort"
	"strings"
)

type selectedAsset = installedlayout.Asset
type descriptor = installedlayout.Descriptor

func generate(ctx context.Context, role role, image staging.Inspection, inputs map[string]string) ([]byte, error) {
	switch role.Operation {
	case "retire":
		return nil, nil
	case "inert-data":
		data, ok := image.Assets[role.Source]
		if !ok {
			return nil, conflict("required canonical installation data is missing")
		}
		return data, ctx.Err()
	case "copy":
		data, ok := image.Assets[role.Source]
		if !ok {
			return nil, conflict("required installation source asset is missing")
		}
		if role.Generator == "baseline-project-substitution" {
			return project(ctx, data, inputs)
		}
		return data, ctx.Err()
	case "runtime-control":
		switch role.Path {
		case ".factory-version":
			return []byte("ref=" + image.Runtime.Version + "\ncommit=" + image.Manifest.SourceRevision + "\n"), nil
		case ".factory/bin/runtime.manifest":
			return image.RuntimeManifest, nil
		case ".factory/bin/source.json":
			return image.Source, nil
		case ".factory/bin/factory-runtime", ".factory/installation.current":
			return nil, nil
		}
	case "generate":
		switch role.Generator {
		case "installed-launcher":
			return []byte(fmt.Sprintf("#!/bin/bash\n# Factory installation %s %s %s\nset -e\n_factory_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")\" && pwd -P)\"\nexec \"$_factory_root/.factory/bin/factory-runtime\" --installed %s %s %s -- \"$@\"\n", image.Runtime.Version, image.Manifest.SourceRevision, image.Runtime.SHA256, image.Runtime.Version, image.Manifest.SourceRevision, image.Runtime.SHA256)), nil
		case "native-command-adapter":
			name := strings.TrimSuffix(strings.TrimPrefix(path.Base(role.Path), "factory-"), ".sh")
			parent := ".."
			if role.Path == "scripts/selftest/run.sh" {
				name = "selftest"
				parent = "../.."
			}
			return []byte(fmt.Sprintf("#!/bin/bash\n# Factory installation %s %s %s\nset -e\n_factory_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/%s\" && pwd -P)\"\nexec \"$_factory_root/factory\" %s \"$@\"\n", image.Runtime.Version, image.Manifest.SourceRevision, image.Runtime.SHA256, parent, name)), nil
		case "sourceable-go-bridge":
			if role.Path == "scripts/lib/budget-config.sh" {
				data, found := image.Assets[role.Path]
				if !found {
					return nil, conflict("required budget bridge source is missing")
				}
				return data, nil
			}
			reader, found := image.Assets["runtime/shell/readers.sh"]
			if !found {
				return nil, conflict("required native bridge source is missing")
			}
			source := string(reader)
			start := strings.Index(source, "_factory_reader_call() (")
			end := strings.Index(source, "\nfactory_config_file()")
			if start < 0 || end < start {
				return nil, conflict("unsupported native bridge source")
			}
			source = source[:start] + `_factory_reader_call() (
  _factory_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)" || return
  /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" FACTORY_BRIDGE_PROTOCOL=1 "$_factory_root/factory" "$@"
)
` + source[end:]
			if role.Path == "scripts/lib/config.sh" {
				config, found := image.Assets["runtime/shell/config.sh"]
				if !found {
					return nil, conflict("required native configuration bridge source is missing")
				}
				text := string(config)
				start := strings.Index(text, "_factory_config_call() (")
				end := strings.Index(text, "\n_factory_config_bad_plan()")
				if start < 0 || end < start {
					return nil, conflict("unsupported native configuration bridge")
				}
				text = text[:start] + "_factory_config_call() { _factory_reader_call \"$@\"; }\n" + text[end:]
				source += "\n" + text
			}
			return []byte("# Factory installation " + image.Runtime.Version + " " + image.Manifest.SourceRevision + " " + image.Runtime.SHA256 + "\n" + source), ctx.Err()
		}
	}
	return nil, conflict("unrecognized reviewed installation role")
}
func descriptorBytes(image staging.Inspection, actions []Action) ([]byte, error) {
	result := descriptor{SchemaVersion: 1, Kind: "go-hybrid-v1", Version: image.Runtime.Version, Revision: image.Manifest.SourceRevision, Target: image.Runtime.Target, RuntimeSHA256: image.Runtime.SHA256, Assets: []selectedAsset{}}
	for _, action := range actions {
		active := false
		for _, row := range roles {
			if row.Path == action.Path && row.Operation != "retire" {
				active = true
				break
			}
		}
		if active && action.After != nil && action.Path != ".factory/installation.current" && action.Path != ".factory-version" {
			result.Assets = append(result.Assets, selectedAsset{Path: action.Path, Mode: action.After.Mode, SHA256: action.After.SHA256, Bytes: action.After.Bytes})
		}
	}
	sort.Slice(result.Assets, func(i, j int) bool { return result.Assets[i].Path < result.Assets[j].Path })
	data, err := json.Marshal(result)
	return append(data, '\n'), err
}
