# operator-go/pkg/config - Configuration Generation and Merging

**Parent:** [../AGENTS.md](../AGENTS.md)

Config-file serialization for several formats, and the layered merge of CRD override layers.

> Product **logging** configuration (Log4j, Log4j2, Logback, Python) lives in `pkg/productlogging`,
> not here.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `format.go` | `ConfigFormat` interface — `Marshal(map[string]string) (string, error)` **and** `Unmarshal(string) (map[string]string, error)`; both are required of every adapter |
| `generator.go` | `ConfigGenerator` (single fixed format: `Generate`/`Parse`/`GenerateFiles`) and `MultiFormatConfigGenerator` (extension→adapter dispatch: `RegisterFormat`, `RegisterDefaultFormats`, `Generate`, `GenerateFiles`) |
| `xml_adapter.go` | Hadoop-style XML adapter (`.xml`) |
| `properties_adapter.go` | Java properties adapter (`.properties`) |
| `yaml_adapter.go` | Flat-mapping YAML adapter (`.yaml`, `.yml`), backed by `gopkg.in/yaml.v3` |
| `env_adapter.go` | dotenv-style adapter (`.env`) |
| `ini_adapter.go` | INI adapter (`.ini`) |
| `merger.go` | `ConfigMerger`, `MergedConfig`, `MergeStrategy` — the layered override merge |

## Format Registration

`RegisterDefaultFormats()` registers **`.xml`, `.properties`, `.yaml`, `.yml`, `.env` and `.ini`**.
No default format needs a separate opt-in; `RegisterFormat` is for adding a product's own extension
or replacing a built-in adapter.

## Adapter Contract

Adapters validate their input and return an error rather than emitting output the target parser
would misread:

- **YAML** only round-trips a *flat* mapping. `Unmarshal` rejects a nested value, a sequence or
  scalar document root, and complex keys. `Marshal` quotes values that would otherwise change type
  (`true`, `123`) and emits multi-line values as block scalars; keys are sorted.
- **Env** rejects a key that is not a shell variable name (`^[A-Za-z_][A-Za-z0-9_]*$`). `\n`, `\r`
  and `\t` in values are dotenv-style escapes, so a multi-line value is *not* byte-faithful when a
  POSIX shell sources the file; `$` and backticks are escaped in quoted values.
- **INI** rejects a key or value containing a line break, a key containing `=` or `:`, and a key
  starting with `[`, `#` or `;`.
- **Properties** round-trips exactly. Separators, comment markers and the edge whitespace of a key
  or value are escaped on write and honoured on read, so keys such as `a ` or `#a` come back
  unchanged; a value ending in an escaped backslash does not swallow the following line.
  Whitespace that is *not* escaped is layout (indentation, padding around the separator) and is
  dropped, as a hand-written file expects.

## Merge Semantics

`ConfigMerger.Merge(...*v1alpha1.OverridesSpec)` is variadic and folds the layers left to right in
**increasing** precedence. The conventional order is product config (lowest) → role overrides →
role group overrides (highest), so a value a user sets in the CRD always wins over a product
default. `nil` layers are skipped.

| Field | Strategy |
|---|---|
| `configOverrides`, `envOverrides` | deep merge (per file, then per key) |
| `cliOverrides` | `ConfigMerger.SliceMergeStrategy` |
| `podOverrides` | Kubernetes strategic merge patch on `PodTemplateSpec` |

Two contracts worth knowing before relying on them:

- **`SliceMergeStrategy` defaults to `MergeStrategyReplace`.** `MergeStrategyAppend` exists and is
  honoured by `Merge`, but it is only reachable by constructing a `ConfigMerger` yourself:
  `GenericReconciler` builds its merger with `config.NewConfigMerger()` and exposes no knob, so the
  framework path is always Replace.
- **An empty slice means "unset", not "clear".** `mergeSlices` returns the base untouched when the
  override has length 0, so a role group cannot erase the CLI arguments its role set — only a
  non-empty override replaces (or, under Append, extends) them.

`MergedConfig.PodOverrideErrors []error` collects the `podOverrides` layers that could not be
applied (raw JSON that does not decode into a `PodTemplateSpec`, or a failed strategic merge
patch). `Merge` has no error return, so a bad layer is dropped from the result *and* recorded here;
`Clone` copies the slice. Callers are expected to surface a non-empty slice —
`GenericReconciler` logs each entry and emits a `PodOverrideIgnored` Warning event on the CR.

## Working Instructions

1. **Adding a new format:** create a `*_adapter.go` implementing **both** `Marshal` and
   `Unmarshal`, then register it (`RegisterFormat(".ext", NewXAdapter())`), and add it to
   `RegisterDefaultFormats` only if every product should get it.
2. **Generating configs:** use `MultiFormatConfigGenerator`; it selects the adapter from the file
   extension.
3. **Merging configs:** use `ConfigMerger.Merge` and always check `MergedConfig.PodOverrideErrors`
   before treating the result as the user's intent.
