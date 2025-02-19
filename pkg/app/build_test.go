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

package app

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tensorchord/envd/pkg/docker"
	"github.com/tensorchord/envd/pkg/home"
)

var _ = Describe("build command", func() {
	buildContext := "testdata"
	args := []string{
		"envd.test", "--debug", "build", "--path", buildContext,
	}
	BeforeEach(func() {
		Expect(home.Initialize()).NotTo(HaveOccurred())
		app := New()
		err := app.Run([]string{"envd.test", "--debug", "bootstrap"})
		Expect(err).NotTo(HaveOccurred())
		cli, err := docker.NewClient(context.TODO())
		Expect(err).NotTo(HaveOccurred())
		_, err = cli.Destroy(context.TODO(), buildContext)
		Expect(err).NotTo(HaveOccurred())
	})
	When("given the right arguments", func() {
		It("should build successfully", func() {
			app := New()
			err := app.Run(args)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
