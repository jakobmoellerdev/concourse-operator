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
- [JobStatus](#jobstatus)
- [PipelineJobStatus](#pipelinejobstatus)

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
Creating a Build with no BuildID triggers a new Concourse build once.
Setting BuildID adopts and watches an existing Concourse build.



_Appears in:_
- [Build](#build)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `jobRef` _[LocalObjectReference](#localobjectreference)_ | JobRef references the Job that this build belongs to. |  | Optional: \{\} <br /> |
| `buildID` _integer_ | BuildID adopts an existing Concourse build instead of creating a new one. |  | Minimum: 1 <br />Optional: \{\} <br /> |
| `canceled` _boolean_ | Canceled requests abort of a non-terminal build. Ignored once the build<br />has reached a terminal state. |  | Optional: \{\} <br /> |
| `ttlSecondsAfterFinished` _integer_ | TTLSecondsAfterFinished is how long to keep this Build after it reaches a<br />terminal state. Zero or unset means keep until history limits prune it. |  | Minimum: 0 <br />Optional: \{\} <br /> |


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
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#duration-v1-meta)_ | Duration is the elapsed time between StartTime and EndTime. |  | Optional: \{\} <br /> |
| `apiURL` _string_ | APIURL is the Concourse API path for this build. |  | Optional: \{\} <br /> |
| `createdBy` _string_ | CreatedBy is the user or system that triggered the build. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### ConfigMapKeyRef



ConfigMapKeyRef references a key within a ConfigMap.



_Appears in:_
- [PipelineConfig](#pipelineconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the ConfigMap. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | Key within the ConfigMap. |  | MinLength: 1 <br />Required: \{\} <br /> |


### Instance



Instance represents a connection to a Concourse CI server.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Instance` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[InstanceSpec](#instancespec)_ |  |  | Required: \{\} <br /> |
| `status` _[InstanceStatus](#instancestatus)_ |  |  | Optional: \{\} <br /> |


### InstanceAuth



InstanceAuth configures authentication to the Concourse ATC.
Exactly one of Password or Token must be set.



_Appears in:_
- [InstanceSpec](#instancespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `password` _[PasswordGrant](#passwordgrant)_ | Password configures an OAuth2 password grant (local user). |  | Optional: \{\} <br /> |
| `token` _[TokenAuth](#tokenauth)_ | Token configures bearer token authentication. |  | Optional: \{\} <br /> |


### InstanceSpec



InstanceSpec defines the desired state of Instance.



_Appears in:_
- [Instance](#instance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the base URL of the Concourse ATC, e.g. https://ci.example.com. |  | Format: uri <br />MaxLength: 2048 <br />Pattern: `^https?://.+` <br />Required: \{\} <br /> |
| `auth` _[InstanceAuth](#instanceauth)_ | Auth configures authentication to the ATC. Required. |  | Required: \{\} <br /> |
| `tls` _[TLSConfig](#tlsconfig)_ | TLS configures TLS settings for the connection. |  | Optional: \{\} <br /> |
| `allowedNamespaces` _string array_ | AllowedNamespaces lists namespaces whose Team/Worker CRs may reference this<br />Instance. Empty means only the Instance's own namespace is allowed. |  | Optional: \{\} <br /> |
| `healthProbeInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#duration-v1-meta)_ | HealthProbeInterval is how often the controller probes this instance. | 5m | Optional: \{\} <br /> |


### InstanceStatus



InstanceStatus defines the observed state of Instance.



_Appears in:_
- [Instance](#instance)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `version` _string_ | Version is the Concourse server version string. |  | Optional: \{\} <br /> |
| `workerVersion` _string_ | WorkerVersion is the minimum worker binary version required by this instance. |  | Optional: \{\} <br /> |
| `clusterName` _string_ | ClusterName is the human-readable cluster name reported by Concourse. |  | Optional: \{\} <br /> |
| `externalURL` _string_ | ExternalURL is the externally-visible URL reported by the Concourse server. |  | Optional: \{\} <br /> |
| `workerCount` _integer_ | WorkerCount is the total number of registered workers. |  | Optional: \{\} <br /> |
| `stalledWorkers` _integer_ | StalledWorkers is the number of workers in the "stalled" state. |  | Optional: \{\} <br /> |
| `runningWorkers` _integer_ | RunningWorkers is the number of workers in the "running" state. |  | Optional: \{\} <br /> |
| `landingWorkers` _integer_ | LandingWorkers is the number of workers in the "landing" state. |  | Optional: \{\} <br /> |
| `authSecretGeneration` _integer_ | AuthSecretGeneration is the generation of the Secret last used for auth. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### Job



Job manages a job within a Concourse pipeline (pause/unpause).
Create a Build CR to trigger a new build.





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
| `jobName` _string_ | JobName is the job name within the pipeline. Defaults to metadata.name. |  | MaxLength: 100 <br />MinLength: 1 <br />Pattern: `^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$` <br />Optional: \{\} <br /> |
| `paused` _boolean_ | Paused sets whether the job should be paused in Concourse. |  | Optional: \{\} <br /> |
| `successfulBuildsHistoryLimit` _integer_ | SuccessfulBuildsHistoryLimit is how many succeeded Build CRs to keep. | 3 | Minimum: 0 <br />Optional: \{\} <br /> |
| `failedBuildsHistoryLimit` _integer_ | FailedBuildsHistoryLimit is how many failed/errored/aborted Build CRs to keep. | 3 | Minimum: 0 <br />Optional: \{\} <br /> |


### JobStatus



JobStatus defines the observed state of Job.



_Appears in:_
- [Job](#job)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `resolvedName` _string_ | ResolvedName is the Concourse job name after defaulting. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused reflects the actual pause state in Concourse. Nil means not yet observed. |  | Optional: \{\} <br /> |
| `nextBuildName` _string_ | NextBuildName is the name of the most recently created Build CR. |  | Optional: \{\} <br /> |
| `lastBuildID` _integer_ | LastBuildID is the Concourse ID of the last finished build. |  | Optional: \{\} <br /> |
| `lastBuildStatus` _[BuildPhase](#buildphase)_ | LastBuildStatus is the status of the last finished build. |  | Enum: [pending started succeeded failed errored aborted] <br />Optional: \{\} <br /> |
| `lastBuildTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastBuildTime is when the last finished build ended. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### LocalObjectReference



LocalObjectReference names a resource. When Namespace is empty the referent
is resolved in the referring object's namespace.



_Appears in:_
- [BuildSpec](#buildspec)
- [JobSpec](#jobspec)
- [PipelineSpec](#pipelinespec)
- [ResourceSpec](#resourcespec)
- [TeamSpec](#teamspec)
- [WorkerSpec](#workerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the referent. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the referent. Empty means the same namespace as the referring object. |  | MinLength: 1 <br />Optional: \{\} <br /> |


### PasswordGrant



PasswordGrant holds local-user credentials used for an OAuth2 password grant
against Concourse's sky/issuer token endpoint.



_Appears in:_
- [InstanceAuth](#instanceauth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username for the password grant. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `passwordRef` _[SecretKeySelector](#secretkeyselector)_ | PasswordRef references the Secret key containing the password. |  | Required: \{\} <br /> |
| `clientID` _string_ | ClientID is the OAuth2 client ID. Defaults to the well-known fly client. |  | Optional: \{\} <br /> |
| `clientSecretRef` _[SecretKeySelector](#secretkeyselector)_ | ClientSecretRef references the Secret key containing the OAuth2 client secret.<br />When unset, the well-known fly client secret is used. |  | Optional: \{\} <br /> |


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


### PipelineJobStatus



PipelineJobStatus is the observed state of one job in a pipeline.



_Appears in:_
- [PipelineStatus](#pipelinestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the Concourse job name. |  |  |
| `paused` _boolean_ | Paused reflects the pause state in Concourse. |  | Optional: \{\} <br /> |
| `lastBuildStatus` _[BuildPhase](#buildphase)_ | LastBuildStatus is the status of the last finished build, if any. |  | Enum: [pending started succeeded failed errored aborted] <br />Optional: \{\} <br /> |


### PipelineResourceStatus



PipelineResourceStatus is the observed state of one resource in a pipeline.



_Appears in:_
- [PipelineStatus](#pipelinestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the Concourse resource name. |  |  |
| `pinned` _boolean_ | Pinned indicates whether the resource is currently pinned. |  | Optional: \{\} <br /> |
| `type` _string_ | Type is the Concourse resource type. |  | Optional: \{\} <br /> |


### PipelineSpec



PipelineSpec defines the desired state of Pipeline.



_Appears in:_
- [Pipeline](#pipeline)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `teamRef` _[LocalObjectReference](#localobjectreference)_ | TeamRef references the Team this pipeline belongs to. |  | Required: \{\} <br /> |
| `pipelineName` _string_ | PipelineName is the name in Concourse. Defaults to metadata.name. |  | MaxLength: 100 <br />MinLength: 1 <br />Pattern: `^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$` <br />Optional: \{\} <br /> |
| `config` _[PipelineConfig](#pipelineconfig)_ | Config defines the pipeline configuration source. |  | Required: \{\} <br /> |
| `vars` _[PipelineVar](#pipelinevar) array_ | Vars are pipeline variables passed at set-pipeline time. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused sets the pipeline pause state in Concourse. |  | Optional: \{\} <br /> |
| `exposed` _boolean_ | Exposed makes the pipeline publicly visible without authentication. |  | Optional: \{\} <br /> |
| `reclaimPolicy` _[ReclaimPolicy](#reclaimpolicy)_ | ReclaimPolicy controls whether the Concourse pipeline is deleted when this<br />CR is removed. Defaults to Delete. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


### PipelineStatus



PipelineStatus defines the observed state of Pipeline.



_Appears in:_
- [Pipeline](#pipeline)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `pipelineID` _integer_ | PipelineID is the Concourse-assigned pipeline ID. |  | Optional: \{\} <br /> |
| `resolvedName` _string_ | ResolvedName is the Concourse pipeline name after defaulting. |  | Optional: \{\} <br /> |
| `configHash` _string_ | ConfigHash is the SHA-256 hash of the last successfully applied pipeline<br />YAML plus interpolated vars. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | Paused reflects the actual pause state in Concourse. Nil means not yet observed. |  | Optional: \{\} <br /> |
| `exposed` _boolean_ | Exposed reflects the actual exposed state in Concourse. Nil means not yet observed. |  | Optional: \{\} <br /> |
| `lastUpdated` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastUpdated is the timestamp when Concourse last recorded a change to this pipeline. |  | Optional: \{\} <br /> |
| `groupCount` _integer_ | GroupCount is the number of job groups defined in the pipeline. |  | Optional: \{\} <br /> |
| `jobs` _[PipelineJobStatus](#pipelinejobstatus) array_ | Jobs lists jobs observed from the applied pipeline config. |  | Optional: \{\} <br /> |
| `resources` _[PipelineResourceStatus](#pipelineresourcestatus) array_ | Resources lists resources observed from the applied pipeline config. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### PipelineVar



PipelineVar defines a variable passed to fly set-pipeline -v/-l.
Exactly one of Value or ValueFrom must be set.



_Appears in:_
- [PipelineSpec](#pipelinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the variable. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `value` _string_ | Value is the plain-text value. Mutually exclusive with ValueFrom. |  | Optional: \{\} <br /> |
| `valueFrom` _[SecretKeySelector](#secretkeyselector)_ | ValueFrom sources the value from a Secret. Mutually exclusive with Value. |  | Optional: \{\} <br /> |


### ReclaimPolicy

_Underlying type:_ _string_

ReclaimPolicy controls what happens to the corresponding Concourse object
when the Kubernetes resource is deleted.

_Validation:_
- Enum: [Delete Orphan]

_Appears in:_
- [PipelineSpec](#pipelinespec)
- [TeamSpec](#teamspec)

| Value | Description |
| --- | --- |
| `Delete` | ReclaimDelete deletes the Concourse object when the CR is removed.<br /> |
| `Orphan` | ReclaimOrphan leaves the Concourse object in place when the CR is removed.<br /> |


### Resource



Resource is a projection of one named resource inside a Pipeline.
It manages pin/unpin and operator-driven checks.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Resource` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[ResourceSpec](#resourcespec)_ |  |  | Required: \{\} <br /> |
| `status` _[ResourceStatus](#resourcestatus)_ |  |  | Optional: \{\} <br /> |


### ResourceSpec



ResourceSpec defines the desired state of a pipeline resource projection.
The CR does not define the Concourse resource (type/source live in the
Pipeline config); it pins and checks an existing named resource.



_Appears in:_
- [Resource](#resource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pipelineRef` _[LocalObjectReference](#localobjectreference)_ | PipelineRef references the Pipeline that owns this resource. |  | Required: \{\} <br /> |
| `resourceName` _string_ | ResourceName is the resource name within the pipeline. Defaults to metadata.name. |  | MaxLength: 100 <br />MinLength: 1 <br />Pattern: `^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$` <br />Optional: \{\} <br /> |
| `pinnedVersion` _object (keys:string, values:string)_ | PinnedVersion pins the resource to a specific version.<br />When set, the controller looks up the version ID and calls PinResourceVersion.<br />When cleared, the controller calls UnpinResource. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `checkInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#duration-v1-meta)_ | CheckInterval triggers a Concourse resource check at this interval.<br />When unset, no automatic checks are triggered by the operator. |  | Optional: \{\} <br /> |


### ResourceStatus



ResourceStatus defines the observed state of Resource.



_Appears in:_
- [Resource](#resource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `resolvedName` _string_ | ResolvedName is the Concourse resource name after defaulting. |  | Optional: \{\} <br /> |
| `lastChecked` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastChecked is the timestamp of the last resource check. |  | Optional: \{\} <br /> |
| `latestVersion` _object (keys:string, values:string)_ | LatestVersion is the most recent version of the resource. |  | Optional: \{\} <br /> |
| `pinnedVersionID` _integer_ | PinnedVersionID is the Concourse resource version ID that is pinned. |  | Optional: \{\} <br /> |
| `pinned` _boolean_ | Pinned indicates whether the resource is currently pinned in Concourse. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### SecretKeySelector



SecretKeySelector references a key in a Kubernetes Secret.



_Appears in:_
- [PasswordGrant](#passwordgrant)
- [PipelineVar](#pipelinevar)
- [TLSConfig](#tlsconfig)
- [TokenAuth](#tokenauth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | Key within the Secret. |  | MinLength: 1 <br />Required: \{\} <br /> |


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
| `teamName` _string_ | TeamName is the team name in Concourse. Defaults to metadata.name. |  | MaxLength: 100 <br />MinLength: 1 <br />Pattern: `^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$` <br />Optional: \{\} <br /> |
| `roles` _[TeamRole](#teamrole) array_ | Roles defines auth role bindings for the team. |  | Optional: \{\} <br /> |
| `allowDestroy` _boolean_ | AllowDestroy permits deleting the reserved Concourse "main" team.<br />Defaults to false. Destroying main is almost always a mistake. |  | Optional: \{\} <br /> |
| `reclaimPolicy` _[ReclaimPolicy](#reclaimpolicy)_ | ReclaimPolicy controls whether the Concourse team is deleted when this<br />CR is removed. Defaults to Delete. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


### TeamStatus



TeamStatus defines the observed state of Team.



_Appears in:_
- [Team](#team)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `teamID` _integer_ | TeamID is the Concourse-assigned numeric team ID. |  | Optional: \{\} <br /> |
| `resolvedName` _string_ | ResolvedName is the Concourse team name after defaulting. |  | Optional: \{\} <br /> |
| `lastApplied` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastApplied is the timestamp of the last successful CreateOrUpdate. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


### TokenAuth



TokenAuth holds a bearer token reference.



_Appears in:_
- [InstanceAuth](#instanceauth)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tokenRef` _[SecretKeySelector](#secretkeyselector)_ | TokenRef references the Secret key containing the bearer token. |  | Required: \{\} <br /> |


### Worker



Worker observes and manages a worker in a Concourse instance.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `concourse-ci.org/v1alpha1` | | |
| `kind` _string_ | `Worker` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[WorkerSpec](#workerspec)_ |  |  | Required: \{\} <br /> |
| `status` _[WorkerStatus](#workerstatus)_ |  |  | Optional: \{\} <br /> |


### WorkerLifecycle

_Underlying type:_ _string_

WorkerLifecycle is the desired lifecycle of a worker.

_Validation:_
- Enum: [Running Draining Removed]

_Appears in:_
- [WorkerSpec](#workerspec)

| Value | Description |
| --- | --- |
| `Running` | WorkerLifecycleRunning leaves the worker in the pool (no-op; workers self-register).<br /> |
| `Draining` | WorkerLifecycleDraining lands the worker (graceful drain).<br /> |
| `Removed` | WorkerLifecycleRemoved prunes the worker from the pool.<br /> |


### WorkerPhase

_Underlying type:_ _string_

WorkerPhase is the observed Concourse worker state.

_Validation:_
- Enum: [running landing landed retiring stalled missing]

_Appears in:_
- [WorkerStatus](#workerstatus)

| Value | Description |
| --- | --- |
| `running` |  |
| `landing` |  |
| `landed` |  |
| `retiring` |  |
| `stalled` |  |
| `missing` |  |


### WorkerSpec



WorkerSpec defines the desired state of Worker.



_Appears in:_
- [Worker](#worker)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `instanceRef` _[LocalObjectReference](#localobjectreference)_ | InstanceRef references the Instance this worker belongs to. |  | Required: \{\} <br /> |
| `workerName` _string_ | WorkerName is the unique name of the worker in Concourse.<br />Defaults to metadata.name. |  | MaxLength: 100 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `lifecycle` _[WorkerLifecycle](#workerlifecycle)_ | Lifecycle is the desired worker lifecycle.<br />- Running: leave the worker in the pool (default)<br />- Draining: land the worker (graceful drain)<br />- Removed: prune the worker from the pool | Running | Enum: [Running Draining Removed] <br />Optional: \{\} <br /> |


### WorkerStatus



WorkerStatus defines the observed state of Worker.



_Appears in:_
- [Worker](#worker)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `phase` _[WorkerPhase](#workerphase)_ | Phase is the current state of the worker as reported by Concourse. |  | Enum: [running landing landed retiring stalled missing] <br />Optional: \{\} <br /> |
| `resolvedName` _string_ | ResolvedName is the Concourse worker name after defaulting. |  | Optional: \{\} <br /> |
| `platform` _string_ | Platform is the worker platform (e.g. linux, darwin). |  | Optional: \{\} <br /> |
| `tags` _string array_ | Tags are the worker tags. |  | Optional: \{\} <br /> |
| `activeContainers` _integer_ | ActiveContainers is the number of containers running on the worker. |  | Optional: \{\} <br /> |
| `activeVolumes` _integer_ | ActiveVolumes is the number of volumes on the worker. |  | Optional: \{\} <br /> |
| `version` _string_ | Version is the worker binary version. |  | Optional: \{\} <br /> |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | StartTime is when the worker registered with Concourse. |  | Optional: \{\} <br /> |
| `ephemeral` _boolean_ | Ephemeral indicates whether the worker is ephemeral (auto-removed on disconnect). |  | Optional: \{\} <br /> |
| `team` _string_ | Team is the Concourse team this worker is scoped to; empty means global worker. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last generation that was reconciled. |  | Optional: \{\} <br /> |


