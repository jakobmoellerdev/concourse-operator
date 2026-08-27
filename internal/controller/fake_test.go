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

package controller

import (
	"io"
	"net/http"
	"time"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
)

// fakeTeam implements goconcourse.Team. Only methods called by controllers are
// wired; everything else panics so gaps surface immediately in tests.
type fakeTeam struct {
	name string

	createOrUpdateFn               func(atc.Team) (atc.Team, bool, bool, []goconcourse.ConfigWarning, error)
	destroyTeamFn                  func(string) error
	createOrUpdatePipelineConfigFn func(atc.PipelineRef, string, []byte, bool) (bool, bool, []goconcourse.ConfigWarning, error)
	deletePipelineFn               func(atc.PipelineRef) (bool, error)
	pausePipelineFn                func(atc.PipelineRef) (bool, error)
	unpausePipelineFn              func(atc.PipelineRef) (bool, error)
	exposePipelineFn               func(atc.PipelineRef) (bool, error)
	hidePipelineFn                 func(atc.PipelineRef) (bool, error)
	pipelineFn                     func(atc.PipelineRef) (atc.Pipeline, bool, error)
	pauseJobFn                     func(atc.PipelineRef, string) (bool, error)
	unpauseJobFn                   func(atc.PipelineRef, string) (bool, error)
	createJobBuildFn               func(atc.PipelineRef, string) (atc.Build, error)
	rerunJobBuildFn                func(atc.PipelineRef, string, string) (atc.Build, error)
	setJobBuildCommentFn           func(atc.PipelineRef, string, string, string) (bool, error)
	checkResourceFn                func(atc.PipelineRef, string, atc.Version, bool) (atc.Build, bool, error)
	resourceVersionsFn             func(atc.PipelineRef, string, goconcourse.Page, atc.Version) ([]atc.ResourceVersion, goconcourse.Pagination, bool, error)
	jobFn                          func(atc.PipelineRef, string) (atc.Job, bool, error)
	resourceFn                     func(atc.PipelineRef, string) (atc.Resource, bool, error)
	pinResourceVersionFn           func(atc.PipelineRef, string, int) (bool, error)
	unpinResourceFn                func(atc.PipelineRef, string) (bool, error)
}

func (t *fakeTeam) Name() string       { return t.name }
func (t *fakeTeam) ID() int            { return 0 }
func (t *fakeTeam) Auth() atc.TeamAuth { return nil }
func (t *fakeTeam) ATCTeam() atc.Team  { return atc.Team{Name: t.name} }

func (t *fakeTeam) CreateOrUpdate(team atc.Team) (atc.Team, bool, bool, []goconcourse.ConfigWarning, error) {
	if t.createOrUpdateFn != nil {
		return t.createOrUpdateFn(team)
	}
	panic("fakeTeam.CreateOrUpdate not configured")
}

func (t *fakeTeam) RenameTeam(_, _ string) (bool, []goconcourse.ConfigWarning, error) {
	panic("fakeTeam.RenameTeam not configured")
}

func (t *fakeTeam) DestroyTeam(name string) error {
	if t.destroyTeamFn != nil {
		return t.destroyTeamFn(name)
	}
	panic("fakeTeam.DestroyTeam not configured")
}

func (t *fakeTeam) Pipeline(ref atc.PipelineRef) (atc.Pipeline, bool, error) {
	if t.pipelineFn != nil {
		return t.pipelineFn(ref)
	}
	return atc.Pipeline{}, false, nil
}

func (t *fakeTeam) PipelineBuilds(_ atc.PipelineRef, _ goconcourse.Page) ([]atc.Build, goconcourse.Pagination, bool, error) {
	return nil, goconcourse.Pagination{}, false, nil
}

func (t *fakeTeam) DeletePipeline(ref atc.PipelineRef) (bool, error) {
	if t.deletePipelineFn != nil {
		return t.deletePipelineFn(ref)
	}
	return true, nil
}

func (t *fakeTeam) PausePipeline(ref atc.PipelineRef) (bool, error) {
	if t.pausePipelineFn != nil {
		return t.pausePipelineFn(ref)
	}
	return true, nil
}

func (t *fakeTeam) ArchivePipeline(_ atc.PipelineRef) (bool, error) { return false, nil }

func (t *fakeTeam) UnpausePipeline(ref atc.PipelineRef) (bool, error) {
	if t.unpausePipelineFn != nil {
		return t.unpausePipelineFn(ref)
	}
	return true, nil
}

func (t *fakeTeam) ExposePipeline(ref atc.PipelineRef) (bool, error) {
	if t.exposePipelineFn != nil {
		return t.exposePipelineFn(ref)
	}
	return true, nil
}

func (t *fakeTeam) HidePipeline(ref atc.PipelineRef) (bool, error) {
	if t.hidePipelineFn != nil {
		return t.hidePipelineFn(ref)
	}
	return true, nil
}

func (t *fakeTeam) RenamePipeline(_, _ string) (bool, []goconcourse.ConfigWarning, error) {
	return false, nil, nil
}

func (t *fakeTeam) ListPipelines() ([]atc.Pipeline, error) { return nil, nil }

func (t *fakeTeam) PipelineConfig(_ atc.PipelineRef) (atc.Config, string, bool, error) {
	return atc.Config{}, "", false, nil
}

func (t *fakeTeam) CreateOrUpdatePipelineConfig(ref atc.PipelineRef, ver string, cfg []byte, check bool) (bool, bool, []goconcourse.ConfigWarning, error) {
	if t.createOrUpdatePipelineConfigFn != nil {
		return t.createOrUpdatePipelineConfigFn(ref, ver, cfg, check)
	}
	panic("fakeTeam.CreateOrUpdatePipelineConfig not configured")
}

func (t *fakeTeam) CreatePipelineBuild(_ atc.PipelineRef, _ atc.Plan) (atc.Build, error) {
	return atc.Build{}, nil
}

func (t *fakeTeam) BuildInputsForJob(_ atc.PipelineRef, _ string) ([]atc.BuildInput, bool, error) {
	return nil, false, nil
}

func (t *fakeTeam) Job(ref atc.PipelineRef, name string) (atc.Job, bool, error) {
	if t.jobFn != nil {
		return t.jobFn(ref, name)
	}
	return atc.Job{Name: name}, true, nil
}

func (t *fakeTeam) JobBuild(_ atc.PipelineRef, _, _ string) (atc.Build, bool, error) {
	return atc.Build{}, false, nil
}

func (t *fakeTeam) JobBuilds(_ atc.PipelineRef, _ string, _ goconcourse.Page) ([]atc.Build, goconcourse.Pagination, bool, error) {
	return nil, goconcourse.Pagination{}, false, nil
}

func (t *fakeTeam) CreateJobBuild(ref atc.PipelineRef, jobName string) (atc.Build, error) {
	if t.createJobBuildFn != nil {
		return t.createJobBuildFn(ref, jobName)
	}
	panic("fakeTeam.CreateJobBuild not configured")
}

func (t *fakeTeam) RerunJobBuild(ref atc.PipelineRef, jobName, buildName string) (atc.Build, error) {
	if t.rerunJobBuildFn != nil {
		return t.rerunJobBuildFn(ref, jobName, buildName)
	}
	panic("fakeTeam.RerunJobBuild not configured")
}

func (t *fakeTeam) SetJobBuildComment(ref atc.PipelineRef, jobName, buildName, comment string) (bool, error) {
	if t.setJobBuildCommentFn != nil {
		return t.setJobBuildCommentFn(ref, jobName, buildName, comment)
	}
	panic("fakeTeam.SetJobBuildComment not configured")
}

func (t *fakeTeam) ListJobs(_ atc.PipelineRef) ([]atc.Job, error)         { return nil, nil }
func (t *fakeTeam) ScheduleJob(_ atc.PipelineRef, _ string) (bool, error) { return false, nil }

func (t *fakeTeam) PauseJob(ref atc.PipelineRef, jobName string) (bool, error) {
	if t.pauseJobFn != nil {
		return t.pauseJobFn(ref, jobName)
	}
	return true, nil
}

func (t *fakeTeam) UnpauseJob(ref atc.PipelineRef, jobName string) (bool, error) {
	if t.unpauseJobFn != nil {
		return t.unpauseJobFn(ref, jobName)
	}
	return true, nil
}

func (t *fakeTeam) ClearTaskCache(_ atc.PipelineRef, _, _, _ string) (int64, error) { return 0, nil }

func (t *fakeTeam) Resource(ref atc.PipelineRef, name string) (atc.Resource, bool, error) {
	if t.resourceFn != nil {
		return t.resourceFn(ref, name)
	}
	return atc.Resource{Name: name}, true, nil
}

func (t *fakeTeam) ListResources(_ atc.PipelineRef) ([]atc.Resource, error) { return nil, nil }

func (t *fakeTeam) ListSharedForResource(_ atc.PipelineRef, _ string) (atc.ResourcesAndTypes, bool, error) {
	return atc.ResourcesAndTypes{}, false, nil
}

func (t *fakeTeam) ListSharedForResourceType(_ atc.PipelineRef, _ string) (atc.ResourcesAndTypes, bool, error) {
	return atc.ResourcesAndTypes{}, false, nil
}

func (t *fakeTeam) ResourceTypes(_ atc.PipelineRef) (atc.ResourceTypes, bool, error) {
	return nil, false, nil
}

func (t *fakeTeam) ResourceVersions(ref atc.PipelineRef, name string, page goconcourse.Page, filter atc.Version) ([]atc.ResourceVersion, goconcourse.Pagination, bool, error) {
	if t.resourceVersionsFn != nil {
		return t.resourceVersionsFn(ref, name, page, filter)
	}
	return nil, goconcourse.Pagination{}, false, nil
}

func (t *fakeTeam) ClearResourceVersions(_ atc.PipelineRef, _ string) (int64, error) { return 0, nil }
func (t *fakeTeam) ClearResourceTypeVersions(_ atc.PipelineRef, _ string) (int64, error) {
	return 0, nil
}

func (t *fakeTeam) CheckResource(ref atc.PipelineRef, name string, ver atc.Version, shallow bool) (atc.Build, bool, error) {
	if t.checkResourceFn != nil {
		return t.checkResourceFn(ref, name, ver, shallow)
	}
	return atc.Build{}, true, nil
}

func (t *fakeTeam) CheckResourceType(_ atc.PipelineRef, _ string, _ atc.Version, _ bool) (atc.Build, bool, error) {
	return atc.Build{}, false, nil
}

func (t *fakeTeam) CheckPrototype(_ atc.PipelineRef, _ string, _ atc.Version, _ bool) (atc.Build, bool, error) {
	return atc.Build{}, false, nil
}

func (t *fakeTeam) DisableResourceVersion(_ atc.PipelineRef, _ string, _ int) (bool, error) {
	return false, nil
}

func (t *fakeTeam) EnableResourceVersion(_ atc.PipelineRef, _ string, _ int) (bool, error) {
	return false, nil
}

func (t *fakeTeam) ClearResourceCache(_ atc.PipelineRef, _ string, _ atc.Version) (int64, error) {
	return 0, nil
}

func (t *fakeTeam) PinResourceVersion(ref atc.PipelineRef, name string, id int) (bool, error) {
	if t.pinResourceVersionFn != nil {
		return t.pinResourceVersionFn(ref, name, id)
	}
	return true, nil
}

func (t *fakeTeam) UnpinResource(ref atc.PipelineRef, name string) (bool, error) {
	if t.unpinResourceFn != nil {
		return t.unpinResourceFn(ref, name)
	}
	return true, nil
}
func (t *fakeTeam) SetPinComment(_ atc.PipelineRef, _, _ string) (bool, error) {
	return false, nil
}

func (t *fakeTeam) BuildsWithVersionAsInput(_ atc.PipelineRef, _ string, _ int) ([]atc.Build, bool, error) {
	return nil, false, nil
}

func (t *fakeTeam) BuildsWithVersionAsOutput(_ atc.PipelineRef, _ string, _ int) ([]atc.Build, bool, error) {
	return nil, false, nil
}

func (t *fakeTeam) ListContainers(_ map[string]string) ([]atc.Container, error) { return nil, nil }
func (t *fakeTeam) GetContainer(_ string) (atc.Container, error)                { return atc.Container{}, nil }
func (t *fakeTeam) ListVolumes() ([]atc.Volume, error)                          { return nil, nil }

func (t *fakeTeam) CreateArtifact(_ io.Reader, _ string, _ []string) (atc.WorkerArtifact, error) {
	return atc.WorkerArtifact{}, nil
}
func (t *fakeTeam) GetArtifact(_ int) (io.ReadCloser, error) { return nil, nil }

func (t *fakeTeam) CreateBuild(_ atc.Plan) (atc.Build, error) { return atc.Build{}, nil }

func (t *fakeTeam) Builds(_ goconcourse.Page) ([]atc.Build, goconcourse.Pagination, error) {
	return nil, goconcourse.Pagination{}, nil
}

func (t *fakeTeam) OrderingPipelines(_ []string) error { return nil }
func (t *fakeTeam) OrderingPipelinesWithinGroup(_ string, _ []atc.InstanceVars) error {
	return nil
}

// fakeClient implements goconcourse.Client.
type fakeClient struct {
	team          *fakeTeam
	getInfoFn     func() (atc.Info, error)
	listWorkersFn func() ([]atc.Worker, error)
	landWorkerFn  func(string) error
	pruneWorkerFn func(string) error
	buildFn       func(string) (atc.Build, bool, error)
	abortBuildFn  func(string) error
}

func (c *fakeClient) URL() string              { return "https://fake-concourse.example.com" }
func (c *fakeClient) HTTPClient() *http.Client { return nil }

func (c *fakeClient) Builds(_ goconcourse.Page) ([]atc.Build, goconcourse.Pagination, error) {
	return nil, goconcourse.Pagination{}, nil
}

func (c *fakeClient) Build(id string) (atc.Build, bool, error) {
	if c.buildFn != nil {
		return c.buildFn(id)
	}
	return atc.Build{}, false, nil
}

func (c *fakeClient) BuildEvents(_ string) (goconcourse.Events, error) { return nil, nil }

func (c *fakeClient) BuildResources(_ int) (atc.BuildInputsOutputs, bool, error) {
	return atc.BuildInputsOutputs{}, false, nil
}

func (c *fakeClient) ListBuildArtifacts(_ string) ([]atc.WorkerArtifact, error) { return nil, nil }

func (c *fakeClient) AbortBuild(id string) error {
	if c.abortBuildFn != nil {
		return c.abortBuildFn(id)
	}
	return nil
}

func (c *fakeClient) BuildPlan(_ int) (atc.PublicBuildPlan, bool, error) {
	return atc.PublicBuildPlan{}, false, nil
}

func (c *fakeClient) SaveWorker(_ atc.Worker, _ *time.Duration) (*atc.Worker, error) { return nil, nil }

func (c *fakeClient) ListWorkers() ([]atc.Worker, error) {
	if c.listWorkersFn != nil {
		return c.listWorkersFn()
	}
	return nil, nil
}

func (c *fakeClient) PruneWorker(name string) error {
	if c.pruneWorkerFn != nil {
		return c.pruneWorkerFn(name)
	}
	return nil
}

func (c *fakeClient) LandWorker(name string) error {
	if c.landWorkerFn != nil {
		return c.landWorkerFn(name)
	}
	return nil
}

func (c *fakeClient) GetInfo() (atc.Info, error) {
	if c.getInfoFn != nil {
		return c.getInfoFn()
	}
	return atc.Info{}, nil
}

func (c *fakeClient) GetCLIReader(_, _ string) (io.ReadCloser, http.Header, error) {
	return nil, nil, nil
}

func (c *fakeClient) ListPipelines() ([]atc.Pipeline, error)      { return nil, nil }
func (c *fakeClient) ListAllJobs() ([]atc.Job, error)             { return nil, nil }
func (c *fakeClient) ListTeams() ([]atc.Team, error)              { return nil, nil }
func (c *fakeClient) FindTeam(_ string) (goconcourse.Team, error) { return c.team, nil }

func (c *fakeClient) Team(_ string) goconcourse.Team { return c.team }

func (c *fakeClient) UserInfo() (atc.UserInfo, error)                      { return atc.UserInfo{}, nil }
func (c *fakeClient) ListActiveUsersSince(_ time.Time) ([]atc.User, error) { return nil, nil }
func (c *fakeClient) GetWall() (atc.Wall, error)                           { return atc.Wall{}, nil }
func (c *fakeClient) SetWall(_ atc.Wall) error                             { return nil }
func (c *fakeClient) ClearWall() error                                     { return nil }
func (c *fakeClient) ListComponents() ([]atc.Component, error)             { return nil, nil }
func (c *fakeClient) PauseComponent(_ string) error                        { return nil }
func (c *fakeClient) UnpauseComponent(_ string) error                      { return nil }
func (c *fakeClient) PauseAllComponents() error                            { return nil }
func (c *fakeClient) UnpauseAllComponents() error                          { return nil }
