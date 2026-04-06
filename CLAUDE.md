# Project: operator-go

## Reference Documentation
- **AGENTS.md** — Full project structure, packages, key concepts, and directory layout. Read this first to understand the codebase.
- **docs/architecture.md** — Authoritative design constraints. All implementations must follow the architecture defined here.
- **docs/security.md** — Security architecture (SecretClass, CSI, RBAC, Pod security).

## Development Rules

### Before Committing Code
1. Run `make generate` if you modified any API structs in `pkg/apis/`
2. Run `make lint` — must pass with zero errors
3. Run `make test` — all tests must pass
4. Never commit if lint or tests fail

### After Code Changes
- Always run `make test` to verify nothing is broken
- Always run `make lint` to ensure code quality
- If adding new public interfaces, update AGENTS.md accordingly

### Design Constraints
- Follow the layered architecture defined in `docs/architecture.md`
- Use Go Generics for type safety — no type assertions
- All operations must be idempotent
- Config merging follows the strict merge strategy (Deep Merge for maps, Replace/Append for slices, Strategic Merge Patch for PodTemplate)
