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

## AI Worktree Development Mode

**IMPORTANT**: When making code changes, work in a worktree under `.worktree/`, NOT in the main working directory.

### Workflow
1. Create worktree: `git worktree add .worktree/<branch-name> -b <branch-name>`
2. Work in `.worktree/<branch-name>/` directory
3. Test: `cd .worktree/<branch-name> && make lint && make test`
4. Commit changes in the worktree
5. Push and create PR from the worktree branch
6. Cleanup: `git worktree remove .worktree/<branch-name>`

### Rules
- NEVER modify files directly in the main working directory
- Each task gets its own worktree with a descriptive branch name
- Run `make generate` if API structs are modified
- Run `make lint && make test` before committing
