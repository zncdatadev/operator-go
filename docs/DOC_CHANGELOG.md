# Documentation Changelog

This document tracks all changes made to the SDK documentation.

---

## [2026-06-29]

### Security Documentation (`security.md`)

#### Changed
- Rewrote Section 3.3 (Pod Security Guidelines) to document the framework's single, canonical
  default pod/container `SecurityContext` applied unconditionally by the base role-group handler:
  - Pod-level: `runAsUser=1001`, `runAsGroup=0` (OpenShift-compatible), `fsGroup=1001`,
    `runAsNonRoot=true`, `seccompProfile.type=RuntimeDefault`
  - Container-level: `runAsUser=1001`, `runAsGroup=0`, `runAsNonRoot=true`,
    `allowPrivilegeEscalation=false`, `capabilities.drop=[ALL]`, `seccompProfile.type=RuntimeDefault`

#### Added
- Documented that `MergedConfig.PodOverrides` **REPLACES** the whole `SecurityContext` (no deep
  merge): a product overriding it must restate any hardening fields it wants to keep, and special
  images override the full SecurityContext this way. Noted the `WithoutDefaultSecurityContext()`
  escape hatch that disables the default entirely.

---

## [2026-06-28]

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

#### Changed
- Section 2.5 (Strict Merge Strategy): reframed config merging as an **ordered, variadic layer fold** (`Product Config < Role < RoleGroup`) instead of a fixed two-layer Role↔RoleGroup merge. Clarified that the two-layer merge is the special case with no product layer (backward compatible).
- Section 2.5: corrected the slice-merge description — only `cliOverrides` is a merged slice type (removed the inaccurate "JVM/Volumes").
- Section 1.3 (Overrides) and 3.2.3 (`ConfigMerger`): updated to state the full precedence `Product Config < Role < RoleGroup`.
- Section 4.3.2: scoped `ProductDefaulter` to typed Spec-field defaulting and cross-referenced the new product-config-computation mechanism.

#### Added
- Section 2.6 (Product Config vs. Defaulting): new section drawing a clear boundary between **`ProductDefaulter`** (Webhook; static defaults for typed Spec fields, persisted at admission) and **`ProductConfig`** (reconcile-time *computed* config-file content, merged as the lowest layer, never persisted). Captures the rationale: upgrade propagation and freshness of values derived from live cluster state.

## [2026-03-09]

### Architecture Documentation (`architecture.md`, `architecture_zh.md`)

#### Added
- Added detailed explanation of Role's two configuration sections in Section 1.3 (Terminology Definition):
  - `roleConfig`: Kubernetes-level management controls (e.g., PodDisruptionBudget), Role-scoped only, NOT inherited by RoleGroups
  - `config`: Workload runtime configuration (resources, affinity, logging), serves as defaults for RoleGroups and CAN be inherited and overridden
- Added important note in Overrides terminology: override fields (`configOverrides`, `envOverrides`, `cliOverrides`, `podOverrides`) are **flattened** directly at Role/RoleGroup level, NOT nested under an `overrides` field

#### Changed
- Renamed interface from `RoleConfigExtender` to `RoleExtender` across documentation
- Updated interface description from "configuration extender for parsing and merging differentiated configurations" to "Role extender for extending `role.config` fields with product-specific settings"
- Updated corresponding generic type description from "Generic Config Extender" to "Generic Role Extender"
- Removed JVM arguments from Overrides terminology description (no longer supported)

### Security Documentation (`security.md`)

No changes in this release.

### Examples

#### Added
- Added comprehensive Role-level comments in `crd-base-example.yaml`:
  - Field inheritance explanation (Role → RoleGroup)
  - Override precedence documentation
- Added `roleConfig` section example with PodDisruptionBudget configuration
- Added detailed comments distinguishing `roleConfig` vs `config` sections

#### Changed
- Updated `crd-base-example.yaml` with concrete example values instead of type placeholders:
  - `gracefulShutdownTimeout`: `"30s"`
  - CPU resources: `min: "500m"`, `max: "1"` (Role), `min: "1"`, `max: "2"` (RoleGroup)
  - Memory resources: `limit: "2Gi"` (Role), `limit: "4Gi"` (RoleGroup)

#### Fixed
- Fixed typo in `crd-base-example.yaml`: `affnity` → `affinity`
- Fixed typo in `crd-base-example.yaml`: `StatefuleSets` → `StatefulSets`

### New Files

No new files in this release.

---

## [2025-02-21]

### Architecture Documentation (`architecture.md`)

#### Added
- Added module category table in Section 4 (Core Module Implementation) to organize 14 modules into 5 functional categories
- Added detailed Extension registration, lifecycle, and execution process documentation in Section 4.2
- Added comprehensive health check mechanism description in Section 4.8, including:
  - Check interval: 120 seconds
  - Timeout: 300 seconds
  - Failure handling strategy (Degraded status marking)
  - Controller error handling (no status modification on internal errors)
- Added safety protection mechanisms for orphaned resource cleanup in Section 4.4:
  - Pre-delete validation
  - Safe deletion order
  - PVC preservation by default
- Added concurrency conflict handling for orphaned resource cleanup:
  - Optimistic locking
  - Conflict resolution strategies
  - Status synchronization
- Enriched Section 5 (Design Patterns) with detailed explanations:
  - Interface Segregation Pattern (5.1)
  - Strategy Pattern (5.2)
  - Template Method Pattern (5.3)
  - Singleton Pattern (5.4)
  - Builder Pattern (5.5)
  - Adapter Pattern (5.6)
  - Observer Pattern (5.7)
  - Pattern Summary Table (5.8)

#### Changed
- Updated Kubernetes version requirement from 1.19+ to 1.31+
- Removed `Connection` terminology from Section 1.3 (Terminology Definition) as it's not an abstract concept

#### Fixed
- Unified zookeeper-related terminology to `zookeeperConfigMap` across all examples

### Security Documentation (`security.md`)

No changes in this release.

### Examples

#### Changed
- Updated `crd-hdfs-example.yaml`: Unified zookeeper-related field name
- Updated `crd-hive-example.yaml`: No changes
- Updated `crd-base-example.yaml`: No changes

### New Files

- Added `TODO.md` at project root to track pending issues and improvements
- Added `docs/architecture_zh.md` - Chinese version of architecture documentation
- Added `docs/DOC_CHANGELOG.md` - This changelog file

---

## Template for Future Entries

```markdown
## [YYYY-MM-DD]

### Architecture Documentation (`architecture.md`)

#### Added
- Item 1
- Item 2

#### Changed
- Item 1
- Item 2

#### Fixed
- Item 1
- Item 2

#### Removed
- Item 1

### Security Documentation (`security.md`)

#### Added/Changed/Fixed/Removed
- Items as applicable

### Examples

#### Added/Changed/Fixed/Removed
- Items as applicable

### New Files

- `path/to/new/file` - Description
```

---

## Legend

- **Added**: New features or content
- **Changed**: Modifications to existing features or content
- **Fixed**: Bug fixes or corrections
- **Removed**: Deprecated features or removed content
