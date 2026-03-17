# pg-resource-operator

Kubernetes operator for managing PostgreSQL resources with Custom Resource Definitions (CRDs). It reconciles `PostgresDatabase`, `PostgresRole`, and `PostgresRoleMembership` resources.

## Features

- Declarative management of PostgreSQL databases, roles, and role memberships
- Finalizers to ensure clean deletion

## Prerequisites

- Kubernetes cluster
- PostgreSQL deployment reachable from the operator
- helm (for installation via Helm)

**NB.:** The operator is tested with **PostgreSQL 13**, but should probably work with other versions as well.

## Install via Helm

The Helm chart lives in [helm/chart](helm/chart).

1. Install CRDs and controller:

```bash
helm install pg-resource-operator ./helm/chart \
	--namespace pg-resource-operator \
	--create-namespace
```

or using the released version:

```bash
helm repo add pg-resource-operator https://tarteo.github.io/pg-resource-operator
helm install pg-resource-operator pg-resource-operator/pg-resource-operator \
    --namespace pg-resource-operator \
    --create-namespace
```

and to uninstall:

```bash
helm uninstall pg-resource-operator --namespace pg-resource-operator
```

## Using the CRDs

The operator manages these CRDs:

- `Postgres` — stores PostgreSQL connection information
- `PostgresDatabase` — manages PostgreSQL databases
- `PostgresRole` — manages PostgreSQL roles
- `PostgresRoleMembership` — manages PostgreSQL role memberships

### Create a Postgres resource

```yaml
apiVersion: pg.onestein.nl/v1
kind: Postgres
metadata:
  name: postgres-sample
spec:
  secret:
    name: postgres-secret
  hostKey: host
  portKey: port
  usernameKey: username
  passwordKey: password
  defaultDatabase: postgres
```

### Create a PostgresRole

```yaml
apiVersion: pg.onestein.nl/v1
kind: PostgresRole
metadata:
  name: role-sample
spec:
  postgresRef:
    name: postgres-sample
  name: role-sample
  attributes:
    - CREATEDB  
  passwordSecret:
    name: role-sample-secret
  passwordKey: password
```

### Create a PostgresRoleMembership

```yaml
apiVersion: pg.onestein.nl/v1
kind: PostgresRoleMembership
metadata:
  name: postgresrolemembership-sample
spec:
  postgresRef:
    name: postgres-sample
  role:
    name: role-sample
  member:
    name: postgres
  granted: true
```

### Create a PostgresDatabase

```yaml
apiVersion: pg.onestein.nl/v1
kind: PostgresDatabase
metadata:
  name: database-sample
spec:
  postgresRef:
    name: postgres-sample
  name: database-sample
  encoding: UTF8
  template: template1
  owner: role-sample
  privileges:
    - role:
        name: postgres
      connect: true
      create: true
      temporary: true
    - role:
        secretKeyRef:
          name: role-sample
          key: role
      connect: true
      create: false  # Cannot create new schemas
      temporary: true
    - role: # All other roles
        name: public
      connect: false
      create: false
      temporary: false

```

## Development

### Generate CRDs and deepcopy

```bash
make manifests generate
```

### Build and run locally

```bash
make build
make run
```

## Roadmap

- Add more controllers for other PostgreSQL resources
- Add e2e tests
- Test more PostgreSQL versions
- Delete policy for databases
