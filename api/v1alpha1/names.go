/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

// ResolvedTeamName is the Concourse team name, defaulting to metadata.name.
func ResolvedTeamName(team *Team) string {
	if team.Spec.TeamName != "" {
		return team.Spec.TeamName
	}
	return team.Name
}

// ResolvedPipelineName is the Concourse pipeline name, defaulting to metadata.name.
func ResolvedPipelineName(pipeline *Pipeline) string {
	if pipeline.Spec.PipelineName != "" {
		return pipeline.Spec.PipelineName
	}
	return pipeline.Name
}

// ResolvedJobName is the Concourse job name, defaulting to metadata.name.
func ResolvedJobName(job *Job) string {
	if job.Spec.JobName != "" {
		return job.Spec.JobName
	}
	return job.Name
}

// ResolvedResourceName is the Concourse resource name, defaulting to metadata.name.
func ResolvedResourceName(resource *Resource) string {
	if resource.Spec.ResourceName != "" {
		return resource.Spec.ResourceName
	}
	return resource.Name
}

// ResolvedWorkerName is the Concourse worker name, defaulting to metadata.name.
func ResolvedWorkerName(worker *Worker) string {
	if worker.Spec.WorkerName != "" {
		return worker.Spec.WorkerName
	}
	return worker.Name
}
