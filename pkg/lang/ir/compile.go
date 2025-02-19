// Copyright 2022 The envd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ir

import (
	"context"
	"encoding/json"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/moby/buildkit/client/llb"

	"github.com/tensorchord/envd/pkg/progress/compileui"
	"github.com/tensorchord/envd/pkg/types"
)

func NewGraph() *Graph {
	return &Graph{
		OS:       osDefault,
		Language: languageDefault,
		CUDA:     nil,
		CUDNN:    nil,

		PyPIPackages:   []string{},
		RPackages:      []string{},
		SystemPackages: []string{},
		Exec:           []string{},
		Shell:          shellBASH,
	}
}

var DefaultGraph = NewGraph()

func GPUEnabled() bool {
	return DefaultGraph.GPUEnabled()
}

func Compile(ctx context.Context, cachePrefix string, pub string) (*llb.Definition, error) {
	w, err := compileui.New(ctx, os.Stdout, "auto")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create compileui")
	}
	DefaultGraph.Writer = w
	DefaultGraph.CachePrefix = cachePrefix
	DefaultGraph.PublicKeyPath = pub
	state, err := DefaultGraph.Compile()
	if err != nil {
		return nil, err
	}
	// TODO(gaocegege): Support multi platform.
	def, err := state.Marshal(ctx, llb.LinuxAmd64)
	if err != nil {
		return nil, err
	}
	return def, nil
}

func Labels() (map[string]string, error) {
	return DefaultGraph.Labels()
}

func (g Graph) GPUEnabled() bool {
	return g.CUDA != nil
}

func (g Graph) Labels() (map[string]string, error) {
	labels := make(map[string]string)
	str, err := json.Marshal(g.SystemPackages)
	if err != nil {
		return nil, err
	}
	labels[types.ImageLabelAPT] = string(str)
	str, err = json.Marshal(g.PyPIPackages)
	if err != nil {
		return nil, err
	}
	labels[types.ImageLabelPyPI] = string(str)
	str, err = json.Marshal(g.RPackages)
	if err != nil {
		return nil, err
	}
	labels[types.ImageLabelR] = string(str)
	if g.GPUEnabled() {
		labels[types.ImageLabelGPU] = "true"
		labels[types.ImageLabelCUDA] = *g.CUDA
		if g.CUDNN != nil {
			labels[types.ImageLabelCUDNN] = *g.CUDNN
		}
	}
	labels[types.ImageLabelVendor] = types.ImageVendorEnvd

	return labels, nil
}

func (g Graph) Compile() (llb.State, error) {
	// TODO(gaocegege): Support more OS and langs.
	base := g.compileBase()
	aptStage := g.compileUbuntuAPT(base)
	var merged llb.State
	if g.Language == "r" {
		// TODO(terrytangyuan): Support RStudio local server
		rPackageInstallStage := llb.Diff(aptStage, g.installRPackages(aptStage), llb.WithCustomName("install R packages"))
		merged = llb.Merge([]llb.State{
			aptStage, rPackageInstallStage,
		}, llb.WithCustomName("merging all components into one"))
	} else {
		condaChanelStage := g.compileCondaChannel(aptStage)
		pypiMirrorStage := g.compilePyPIIndex(condaChanelStage)

		g.compileJupyter()
		builtinSystemStage := pypiMirrorStage

		sshStage, err := g.copySSHKey(builtinSystemStage)
		if err != nil {
			return llb.State{}, errors.Wrap(err, "failed to copy ssh keys")
		}
		diffSSHStage := llb.Diff(builtinSystemStage, sshStage, llb.WithCustomName("install ssh keys"))

		// Conda affects shell and python, thus we cannot do it parallelly.
		shellStage, err := g.compileShell(builtinSystemStage)
		if err != nil {
			return llb.State{}, errors.Wrap(err, "failed to compile shell")
		}
		condaStage := llb.Diff(builtinSystemStage,
			g.compileCondaPackages(shellStage),
			llb.WithCustomName("install PyPI packages"))

		pypiStage := llb.Diff(builtinSystemStage,
			g.compilePyPIPackages(builtinSystemStage),
			llb.WithCustomName("install PyPI packages"))
		systemStage := llb.Diff(builtinSystemStage, g.compileSystemPackages(builtinSystemStage),
			llb.WithCustomName("install system packages"))

		if err != nil {
			return llb.State{}, errors.Wrap(err, "failed to copy SSH key")
		}

		vscodeStage, err := g.compileVSCode()
		if err != nil {
			return llb.State{}, errors.Wrap(err, "failed to get vscode plugins")
		}

		if vscodeStage != nil {
			merged = llb.Merge([]llb.State{
				builtinSystemStage, systemStage, condaStage,
				diffSSHStage, pypiStage, *vscodeStage,
			}, llb.WithCustomName("merging all components into one"))
		} else {
			merged = llb.Merge([]llb.State{
				builtinSystemStage, systemStage, condaStage,
				diffSSHStage, pypiStage,
			}, llb.WithCustomName("merging all components into one"))
		}
	}

	// TODO(gaocegege): Support order-based exec.
	run := g.compileRun(merged)
	finalStage, err := g.compileGit(run)
	if err != nil {
		return llb.State{}, errors.Wrap(err, "failed to compile git")
	}
	g.Writer.Finish()
	return finalStage, nil
}
