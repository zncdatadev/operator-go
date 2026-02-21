# Documentation Changelog

This document tracks all changes made to the SDK documentation.

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
