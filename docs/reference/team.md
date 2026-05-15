# Team

Manages a Concourse team and its RBAC role bindings. The operator creates the team if it does not exist and reconciles role bindings on every sync.

## Example

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Team
metadata:
  name: my-team
spec:
  instanceRef:
    name: my-concourse
  teamName: main
  roles:
    - role: owner
      users:
        - local:admin
    - role: viewer
      groups:
        - github:my-org:readers
```

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `instanceRef` | [LocalObjectReference](index.md#localobjectreference) | yes | — | The `Instance` this team belongs to. |
| `teamName` | string | no | `metadata.name` | Name of the team in Concourse. Defaults to the CR name if omitted. |
| `roles` | [][TeamRole](#teamrole) | no | — | RBAC role bindings for this team. |

### TeamRole

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `role` | enum | yes | One of: `owner`, `member`, `pipeline-operator`, `viewer`. |
| `users` | []string | no | User identifiers in the form `provider:username` (e.g. `local:admin`, `github:alice`). |
| `groups` | []string | no | Group identifiers in the form `provider:group` or `provider:org:team`. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []metav1.Condition | `Ready` condition. |
| `teamID` | integer | Numeric team ID assigned by Concourse. |
| `observedGeneration` | int64 | Generation of the spec that produced the current status. |

## Usage notes

- `teamName` defaults to `metadata.name` if not set. If you rename the CR, set `teamName` explicitly to avoid creating a new team.
- The `main` team in Concourse has special restrictions — it cannot be deleted. The operator will reconcile its roles but will not attempt to delete it.
- Removing a `Team` CR does **not** delete the team from Concourse — it only removes the Kubernetes object.
