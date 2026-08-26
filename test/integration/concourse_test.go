//go:build integration

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration_test

import (
	"fmt"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Concourse Integration", func() {

	Describe("Client connectivity", func() {
		It("connects and gets server info", func() {
			info, err := concourseClient.GetInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Version).To(Equal("8.2.1"))
			Expect(info.ClusterName).To(Equal("tutorial"))
		})

		It("lists workers", func() {
			Eventually(func() ([]atc.Worker, error) {
				return concourseClient.ListWorkers()
			}, "60s", "3s").ShouldNot(BeEmpty())
		})
	})

	Describe("Team management", func() {
		const testTeamName = "operator-test-team"

		AfterEach(func() {
			_ = concourseClient.Team(testTeamName).DestroyTeam(testTeamName)
		})

		It("creates and deletes a team", func() {
			team := atc.Team{
				Name: testTeamName,
				Auth: atc.TeamAuth{
					"owner": {"users": {"local:test"}},
				},
			}
			result, created, _, _, err := concourseClient.Team(testTeamName).CreateOrUpdate(team)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())
			Expect(result.Name).To(Equal(testTeamName))

			err = concourseClient.Team(testTeamName).DestroyTeam(testTeamName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("lists teams includes main", func() {
			teams, err := concourseClient.ListTeams()
			Expect(err).NotTo(HaveOccurred())
			names := make([]string, len(teams))
			for i, t := range teams {
				names[i] = t.Name
			}
			Expect(names).To(ContainElement("main"))
		})
	})

	Describe("Pipeline management", func() {
		const (
			testTeamName     = "operator-test-team"
			testPipelineName = "operator-test-pipeline"
		)

		var mainTeam goconcourse.Team

		// Minimal valid Concourse pipeline YAML.
		const minimalPipeline = `jobs:
- name: hello
  plan:
  - task: say-hello
    config:
      platform: linux
      image_resource:
        type: registry-image
        source:
          repository: alpine
      run:
        path: echo
        args: ["Hello from operator test"]
`

		BeforeEach(func() {
			mainTeam = concourseClient.Team(testTeamName)
			team := atc.Team{Name: testTeamName, Auth: atc.TeamAuth{"owner": {"users": {"local:test"}}}}
			_, _, _, _, err := mainTeam.CreateOrUpdate(team)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			team := concourseClient.Team(testTeamName)
			_, _ = team.DeletePipeline(atc.PipelineRef{Name: testPipelineName})
			_ = concourseClient.Team(testTeamName).DestroyTeam(testTeamName)
		})

		It("sets and retrieves a pipeline", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}

			created, updated, warnings, err := team.CreateOrUpdatePipelineConfig(pipelineRef, "", []byte(minimalPipeline), false)
			Expect(err).NotTo(HaveOccurred())
			Expect(created || updated).To(BeTrue())
			Expect(warnings).To(BeEmpty())

			pipeline, found, err := team.Pipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(pipeline.Name).To(Equal(testPipelineName))
		})

		It("pauses and unpauses a pipeline", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}

			_, _, _, err := team.CreateOrUpdatePipelineConfig(pipelineRef, "", []byte(minimalPipeline), false)
			Expect(err).NotTo(HaveOccurred())

			paused, err := team.PausePipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(paused).To(BeTrue())

			p, found, err := team.Pipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(p.Paused).To(BeTrue())

			unpaused, err := team.UnpausePipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(unpaused).To(BeTrue())
		})

		It("deletes a pipeline", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}

			_, _, _, err := team.CreateOrUpdatePipelineConfig(pipelineRef, "", []byte(minimalPipeline), false)
			Expect(err).NotTo(HaveOccurred())

			deleted, err := team.DeletePipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(BeTrue())

			_, found, err := team.Pipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())
		})
	})

	Describe("Resource management", func() {
		const (
			testTeamName     = "operator-test-team"
			testPipelineName = "operator-resource-test"
		)

		const pipelineWithResource = `resources:
- name: concourse-git
  type: git
  source:
    uri: https://github.com/concourse/concourse.git
    branch: master

jobs:
- name: use-resource
  plan:
  - get: concourse-git
`

		BeforeEach(func() {
			mainTeam := concourseClient.Team("main")
			team := atc.Team{Name: testTeamName, Auth: atc.TeamAuth{"owner": {"users": {"local:test"}}}}
			_, _, _, _, err := concourseClient.Team(testTeamName).CreateOrUpdate(team)
			Expect(err).NotTo(HaveOccurred())

			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			_, _, _, err = concourseClient.Team(testTeamName).CreateOrUpdatePipelineConfig(
				pipelineRef, "", []byte(pipelineWithResource), false,
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = concourseClient.Team(testTeamName).UnpausePipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
			_ = mainTeam
		})

		AfterEach(func() {
			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			_, _ = concourseClient.Team(testTeamName).DeletePipeline(pipelineRef)
			_ = concourseClient.Team(testTeamName).DestroyTeam(testTeamName)
		})

		It("lists resource versions (may be empty if check not triggered)", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			versions, _, found, err := team.ResourceVersions(pipelineRef, "concourse-git", goconcourse.Page{Limit: 5}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			// versions may be empty if check hasn't run yet — just verify the call succeeds
			_ = versions
			GinkgoWriter.Printf("resource versions: %d\n", len(versions))
		})

		It("triggers a resource check", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			build, found, err := team.CheckResource(pipelineRef, "concourse-git", nil, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(build.ID).NotTo(BeZero())
			GinkgoWriter.Printf("check triggered: build #%d status=%s\n", build.ID, build.Status)
		})
	})

	Describe("Build management", func() {
		const (
			testTeamName     = "operator-test-team"
			testPipelineName = "operator-build-test"
			testJobName      = "hello"
		)

		const pipelineWithJob = `jobs:
- name: hello
  plan:
  - task: say-hello
    config:
      platform: linux
      image_resource:
        type: registry-image
        source:
          repository: alpine
      run:
        path: echo
        args: ["Hello from operator test"]
`

		BeforeEach(func() {
			team := atc.Team{Name: testTeamName, Auth: atc.TeamAuth{"owner": {"users": {"local:test"}}}}
			_, _, _, _, err := concourseClient.Team(testTeamName).CreateOrUpdate(team)
			Expect(err).NotTo(HaveOccurred())

			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			_, _, _, err = concourseClient.Team(testTeamName).CreateOrUpdatePipelineConfig(
				pipelineRef, "", []byte(pipelineWithJob), false,
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = concourseClient.Team(testTeamName).UnpausePipeline(pipelineRef)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			pipelineRef := atc.PipelineRef{Name: testPipelineName}
			_, _ = concourseClient.Team(testTeamName).DeletePipeline(pipelineRef)
			_ = concourseClient.Team(testTeamName).DestroyTeam(testTeamName)
		})

		It("triggers a job build", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}

			build, err := team.CreateJobBuild(pipelineRef, testJobName)
			Expect(err).NotTo(HaveOccurred())
			Expect(build.ID).NotTo(BeZero())
			Expect(build.JobName).To(Equal(testJobName))
			GinkgoWriter.Printf("build triggered: #%d (%s) status=%s\n", build.ID, build.Name, build.Status)
		})

		It("fetches build status", func() {
			team := concourseClient.Team(testTeamName)
			pipelineRef := atc.PipelineRef{Name: testPipelineName}

			build, err := team.CreateJobBuild(pipelineRef, testJobName)
			Expect(err).NotTo(HaveOccurred())

			buildID := fmt.Sprintf("%d", build.ID)
			fetchedBuild, found, err := concourseClient.Build(buildID)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(fetchedBuild.ID).To(Equal(build.ID))
		})
	})
})
