# AGENTS.md

## Project Overview
`operator-go` is a Golang library and Kubernetes Operator foundation providing shared CRDs and utilities for other operators.
Key features include Database Connections (Mysql, Postgres, Redis), S3 Connections, and Authentication CRDs.

## Development Environment
- **Language**: Go
- **Dependency Management**: Go Modules (`go.mod`)
- **Tooling**: Uses `Makefile` to manage local binaries in `bin/` (controller-gen, golangci-lint, setup-envtest).

## Common Commands
Run these from the project root:

- **Install Dependencies/Tools**: `make` or accessing a tool target (automatically installs tools to `bin/`)
- **Generate Code**: `make generate` (Runs `controller-gen` for DeepCopy methods and other boilerplate)
- **Format Code**: `make fmt` (Runs `go fmt`)
- **Run Static Analysis (Vet)**: `make vet`
- **Lint Code**: `make lint` (Runs `golangci-lint`)
- **Auto-fix Lint Errors**: `make lint-fix`
- **Run Tests**: `make test` (Runs `go test` with coverage, utilizing `setup-envtest` for K8s integration tests)

## Directory Structure
- `pkg/apis/`: Contains Kubernetes API definitions (CRDs).
    - `authentication/`: Authentication related CRDs.
    - `commons/`: Common resources and types (Database connections, etc.).
    - `listeners/`: Listener configurations.
    - `s3/`: S3 connection types.
- `hack/`: Scripts and boilerplate text (e.g. copyright headers).
- `bin/`: Directory for project-local binaries (managed by Makefile, e.g. `controller-gen`, `golangci-lint`).

## Code Style & Conventions
- **Formatting**: Must pass `go fmt`.
- **Linting**: Must pass `golangci-lint`.
- **CRDs**: Uses `kubebuilder` markers (tags) for code generation.
- **Generation**: When modifying API structs in `pkg/apis`, always run `make generate` to update `zz_generated.deepcopy.go` files and other generated artifacts.
