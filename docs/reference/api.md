# API Reference

!!! note "Auto-generated"
    This page is generated from Go source at `api/v1alpha1/`. Do not edit manually — run `make docs-generate` to refresh.

## API Groups
- [concourse-ci.org/v1alpha1](#concourse-ciorgv1alpha1)


## concourse-ci.org/v1alpha1

Package v1alpha1 contains API Schema definitions for the concourse v1alpha1 API group.

### Resource Types
- [Build](#build)
- [Instance](#instance)
- [Job](#job)
- [Pipeline](#pipeline)
- [Resource](#resource)
- [Team](#team)
- [Worker](#worker)



### BasicAuth



BasicAuth holds credentials for HTTP basic authentication.



_Appears in:_
- [InstanceSpec](#instancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username for basic authentication. |  |  |
| `passwordRef` _[SecretKeySelector](#secretkeyselector)_ | PasswordRef references the Secret key containing the password. |  |  |


### Build



Build tracks and optionally triggers a build in Concourse.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Build` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[BuildSpec](#buildspec)_ |  |  | Required: \{\} <br /> |
| `status` _[BuildStatus](#buildstatus)_ |  |  | Optional: \{\} <br /> |


### BuildPhase

_Underlying type:_ _string_

BuildPhase mirrors Concourse build status values.

_Validation:_
- Enum: [pending started succeeded failed errored aborted]

_Appears in:_
- [BuildStatus](#buildstatus)

| Value | Description |
| --- | --- |
| `pending` |  |
| `started` |  |
| `succeeded` |  |
| `failed` |  |
| `errored` |  |
| `aborted` |  |


### BuildSpec



BuildSpec defines the desired state of Build.



_Appears in:_
- [Build](#build)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `jobRef` _[LocalObjectReference](#localobjectreference)_ | JobRef references the Job that triggered this build.<br />Required unless OneOff is true. |  | Optional: \{\} <br /> |
| `oneOff` _boolean_ | OneOff indicates this is a one-off build not tied to a pipeline job.<br />When true, JobRef must be unset. |  | Optional: \{\} <br /> |
| `abort` _boolean_ | Abort signals the controller to abort this build if it is running. |  | Optional: \{\} <br /> |


### BuildStatus



BuildStatus defines the observed state of Build.



_Appears in:_
- [Build](#build)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `buildID` _integer_ | BuildID is the numeric Concourse build ID. |  | Optional: \{\} <br /> |
| `buildName` _string_ | BuildName is the display name of the build (e.g. "42"). |  | Optional: \{\} <br /> |
| `concourseStatus` _[BuildPhase](#buildphase)_ | ConcourseStatus reflects the current Concourse build status. |  | Enum: [pending started succeeded failed errored aborted] <br />Optional: \{\} <br /> |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | StartTime is when the build started. |  | Optional: \{\} <br /> |
| `endTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | EndTime is when the build completed. |  | Optional: \{\} <br /> |
| `apiURL` _string_ | APIURL is the URL to the build on the Concourse web UI. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### ConfigMapKeyRef



ConfigMapKeyRef references a key within a ConfigMap.



_Appears in:_
- [PipelineConfig](#pipelineconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the ConfigMap. |  |  |
| `key` _string_ | Key within the ConfigMap. |  |  |


### Instance



Instance represents a connection to a Concourse CI server.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Instance` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[InstanceSpec](#instancespec)_ |  |  | Required: \{\} <br /> |
| `status` _[InstanceStatus](#instancestatus)_ |  |  | Optional: \{\} <br /> |


### InstanceSpec



InstanceSpec defines the desired state of Instance.



_Appears in:_
- [Instance](#instance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the base URL of the Concourse ATC, e.g. https://ci.example.com. |  | Pattern: `^https?://` <br />Required: \{\} <br /> |
| `basicAuth` _[BasicAuth](#basicauth)_ | BasicAuth configures username/password authentication.<br />Mutually exclusive with TokenAuth. |  | Optional: \{\} <br /> |
| `tokenAuth` _[TokenAuth](#tokenauth)_ | TokenAuth configures bearer token authentication.<br />Mutually exclusive with BasicAuth. |  | Optional: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | TLS configures TLS settings for the connection. |  | Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#duration-v1-meta)_ | Interval is how often the controller reconciles this instance. | 5m | Optional: \{\} <br /> |


### InstanceStatus



InstanceStatus defines the observed state of Instance.



_Appears in:_
- [Instance](#instance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `version` _string_ | Version is the Concourse server version string. |  | Optional: \{\} <br /> |
| `workerCount` _integer_ | WorkerCount is the number of healthy workers in this instance. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### Job



Job manages a job within a Concourse pipeline (pause/unpause, trigger builds).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Job` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[JobSpec](#jobspec)_ |  |  | Required: \{\} <br /> |
| `status` _[JobStatus](#jobstatus)_ |  |  | Optional: \{\} <br /> |


### JobSpec



JobSpec defines the desired state of Job.



_Appears in:_
- [Job](#job)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pipelineRef` _[LocalObjectReference](#localobjectreference)_ | PipelineRef references the Pipeline that owns this job. |  | Required: \{\} <br /> |
| `jobName` _string_ | JobName is the job name within the pipeline. Defaults to metadata.name. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused sets whether the job should be paused in Concourse. |  | Optional: \{\} <br /> |
| `triggerBuild` _boolean_ | TriggerBuild causes the controller to create a Build CR on<br />each spec change, triggering a new build. |  | Optional: \{\} <br /> |


### JobStatus



JobStatus defines the observed state of Job.



_Appears in:_
- [Job](#job)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused reflects the actual pause state in Concourse. |  | Optional: \{\} <br /> |
| `nextBuildName` _string_ | NextBuildName is the name of the next Build CR if one was created. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### LocalObjectReference



LocalObjectReference names a resource in the same namespace.



_Appears in:_
- [BuildSpec](#buildspec)
- [JobSpec](#jobspec)
- [PipelineSpec](#pipelinespec)
- [ResourceSpec](#resourcespec)
- [TeamSpec](#teamspec)
- [WorkerSpec](#workerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |


### Pipeline



Pipeline manages a Concourse pipeline configuration.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Pipeline` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[PipelineSpec](#pipelinespec)_ |  |  | Required: \{\} <br /> |
| `status` _[PipelineStatus](#pipelinestatus)_ |  |  | Optional: \{\} <br /> |


### PipelineConfig



PipelineConfig sources the pipeline YAML configuration.
Exactly one of Inline or ConfigMapRef must be set.



_Appears in:_
- [PipelineSpec](#pipelinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `inline` _string_ | Inline is the pipeline YAML embedded directly in the spec. |  | Optional: \{\} <br /> |
| `configMapRef` _[ConfigMapKeyRef](#configmapkeyref)_ | ConfigMapRef references a ConfigMap key containing the pipeline YAML. |  | Optional: \{\} <br /> |


### PipelineSpec



PipelineSpec defines the desired state of Pipeline.



_Appears in:_
- [Pipeline](#pipeline)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `teamRef` _[LocalObjectReference](#localobjectreference)_ | TeamRef references the Team this pipeline belongs to. |  | Required: \{\} <br /> |
| `pipelineName` _string_ | PipelineName is the name in Concourse. Defaults to metadata.name. |  | Optional: \{\} <br /> |
| `config` _[PipelineConfig](#pipelineconfig)_ | Config defines the pipeline configuration source. |  | Required: \{\} <br /> |
| `vars` _[PipelineVar](#pipelinevar) array_ | Vars are pipeline variables passed at set-pipeline time. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused sets the pipeline pause state in Concourse. |  | Optional: \{\} <br /> |
| `exposed` _boolean_ | Exposed makes the pipeline publicly visible without authentication. |  | Optional: \{\} <br /> |


### PipelineStatus



PipelineStatus defines the observed state of Pipeline.



_Appears in:_
- [Pipeline](#pipeline)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `pipelineID` _integer_ | PipelineID is the Concourse-assigned pipeline ID. |  | Optional: \{\} <br /> |
| `configHash` _string_ | ConfigHash is the SHA-256 hash of the last successfully applied pipeline YAML. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused reflects the actual pause state in Concourse. |  | Optional: \{\} <br /> |
| `exposed` _boolean_ | Exposed reflects the actual exposed state in Concourse. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### PipelineVar



PipelineVar defines a variable passed to fly set-pipeline -v/-l.



_Appears in:_
- [PipelineSpec](#pipelinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the variable. |  |  |
| `value` _string_ | Value is the plain-text value. Mutually exclusive with ValueFrom. |  | Optional: \{\} <br /> |
| `valueFrom` _[SecretKeySelector](#secretkeyselector)_ | ValueFrom sources the value from a Secret. Mutually exclusive with Value. |  | Optional: \{\} <br /> |


### Resource



Resource manages a resource within a Concourse pipeline (pin, check).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Resource` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ResourceSpec](#resourcespec)_ |  |  | Required: \{\} <br /> |
| `status` _[ResourceStatus](#resourcestatus)_ |  |  | Optional: \{\} <br /> |


### ResourceSpec



ResourceSpec defines the desired state of Resource.



_Appears in:_
- [Resource](#resource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pipelineRef` _[LocalObjectReference](#localobjectreference)_ | PipelineRef references the Pipeline that owns this resource. |  | Required: \{\} <br /> |
| `resourceName` _string_ | ResourceName is the resource name within the pipeline. Defaults to metadata.name. |  | Optional: \{\} <br /> |
| `pinnedVersion` _object (keys:string, values:string)_ | PinnedVersion pins the resource to a specific version.<br />When set, the controller calls PinResourceVersion in Concourse.<br />When cleared, the controller calls UnpinResource. |  | Optional: \{\} <br /> |
| `checkInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#duration-v1-meta)_ | CheckInterval triggers a Concourse resource check at this interval.<br />When unset, no automatic checks are triggered by the operator. |  | Optional: \{\} <br /> |


### ResourceStatus



ResourceStatus defines the observed state of Resource.



_Appears in:_
- [Resource](#resource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `lastChecked` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastChecked is the timestamp of the last resource check. |  | Optional: \{\} <br /> |
| `latestVersion` _object (keys:string, values:string)_ | LatestVersion is the most recent version of the resource. |  | Optional: \{\} <br /> |
| `pinnedVersionID` _integer_ | PinnedVersionID is the Concourse resource version ID that is pinned. |  | Optional: \{\} <br /> |
| `pinned` _boolean_ | Pinned indicates whether the resource is currently pinned. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### SecretKeySelector



SecretKeySelector references a key in a Kubernetes Secret.



_Appears in:_
- [BasicAuth](#basicauth)
- [PipelineVar](#pipelinevar)
- [TLSConfig](#tlsconfig)
- [TokenAuth](#tokenauth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  |  |
| `key` _string_ | Key within the Secret. |  |  |


### TLSConfig



TLSConfig configures TLS for the Concourse connection.



_Appears in:_
- [InstanceSpec](#instancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `caRef` _[SecretKeySelector](#secretkeyselector)_ | CARef references a Secret key containing a PEM-encoded CA certificate. |  | Optional: \{\} <br /> |
| `insecureSkipVerify` _boolean_ | InsecureSkipVerify disables TLS certificate verification. |  | Optional: \{\} <br /> |


### Team



Team manages a Concourse team and its auth configuration.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Team` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[TeamSpec](#teamspec)_ |  |  | Required: \{\} <br /> |
| `status` _[TeamStatus](#teamstatus)_ |  |  | Optional: \{\} <br /> |


### TeamRole



TeamRole defines a role binding within a Concourse team.



_Appears in:_
- [TeamSpec](#teamspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `role` _string_ | Role is the Concourse role name (owner, member, pipeline-operator, viewer). |  | Enum: [owner member pipeline-operator viewer] <br /> |
| `users` _string array_ | Users is a list of user identifiers (provider:username). |  | Optional: \{\} <br /> |
| `groups` _string array_ | Groups is a list of group identifiers (provider:group). |  | Optional: \{\} <br /> |


### TeamSpec



TeamSpec defines the desired state of Team.



_Appears in:_
- [Team](#team)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[LocalObjectReference](#localobjectreference)_ | InstanceRef references the Instance this team belongs to. |  | Required: \{\} <br /> |
| `teamName` _string_ | TeamName is the team name in Concourse. Defaults to metadata.name. |  | Optional: \{\} <br /> |
| `roles` _[TeamRole](#teamrole) array_ | Roles defines auth role bindings for the team. |  | Optional: \{\} <br /> |


### TeamStatus



TeamStatus defines the observed state of Team.



_Appears in:_
- [Team](#team)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `teamID` _integer_ | TeamID is the Concourse-assigned numeric team ID. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### TokenAuth



TokenAuth holds a bearer token reference.



_Appears in:_
- [InstanceSpec](#instancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tokenRef` _[SecretKeySelector](#secretkeyselector)_ | TokenRef references the Secret key containing the bearer token. |  |  |


### Worker



Worker manages a worker in a Concourse instance (land, retire, prune).





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Worker` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[WorkerSpec](#workerspec)_ |  |  | Required: \{\} <br /> |
| `status` _[WorkerStatus](#workerstatus)_ |  |  | Optional: \{\} <br /> |


### WorkerDesiredState

_Underlying type:_ _string_

WorkerDesiredState is the desired lifecycle state for a worker.

_Validation:_
- Enum: [active land retire prune]

_Appears in:_
- [WorkerSpec](#workerspec)

| Value | Description |
| --- | --- |
| `active` |  |
| `land` |  |
| `retire` |  |
| `prune` |  |


### WorkerSpec



WorkerSpec defines the desired state of Worker.



_Appears in:_
- [Worker](#worker)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[LocalObjectReference](#localobjectreference)_ | InstanceRef references the Instance this worker belongs to. |  | Required: \{\} <br /> |
| `workerName` _string_ | WorkerName is the unique name of the worker in Concourse. |  | Required: \{\} <br /> |
| `desiredState` _[WorkerDesiredState](#workerdesiredstate)_ | DesiredState sets the lifecycle action to perform on the worker.<br />- active: no-op (workers self-register)<br />- land: gracefully drain and land the worker<br />- retire: retire the worker from the pool<br />- prune: forcibly remove the worker | active | Enum: [active land retire prune] <br />Optional: \{\} <br /> |


### WorkerStatus



WorkerStatus defines the observed state of Worker.



_Appears in:_
- [Worker](#worker)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `actualState` _string_ | ActualState is the current state of the worker as reported by Concourse. |  | Optional: \{\} <br /> |
| `platform` _string_ | Platform is the worker platform (e.g. linux, darwin). |  | Optional: \{\} <br /> |
| `tags` _string array_ | Tags are the worker tags. |  | Optional: \{\} <br /> |
| `activeContainers` _integer_ | ActiveContainers is the number of containers running on the worker. |  | Optional: \{\} <br /> |
| `activeVolumes` _integer_ | ActiveVolumes is the number of volumes on the worker. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


