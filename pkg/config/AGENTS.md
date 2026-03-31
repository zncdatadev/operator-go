# operator-go/pkg/config - Configuration Generation

**Parent:** [../AGENTS.md](../AGENTS.md)
**Generated:** 2026-03-29

Configuration generation and adaptation for various formats (INI, XML, ENV, YAML).

## Key Files

| File | Purpose |
|------|---------|
| `generator.go` | Main config generation logic |
| `format.go` | Format handling utilities |
| `ini_adapter.go` | INI format adapter |
| `xml_adapter.go` | XML format adapter |
| `env_adapter.go` | Environment variable adapter |
| `merger.go` | Config merging logic |

## Working Instructions

1. **Adding a new format:** Create a new adapter file (e.g., `yaml_adapter.go`)
2. **Generating configs:** Use `generator.go` with appropriate adapters
3. **Merging configs:** Use merger utilities for combining multiple configs
