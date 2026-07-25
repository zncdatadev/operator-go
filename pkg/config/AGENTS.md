# operator-go/pkg/config - Configuration Generation and Merging

**Parent:** [../AGENTS.md](../AGENTS.md)

Config-file serialization for several formats, and the layered merge of CRD override layers.

> Product **logging** configuration (Log4j, Log4j2, Logback, Python) lives in `pkg/productlogging`,
> not here.

## Key Files

Every non-test file in this package:

| File | Purpose |
|------|---------|
| `format.go` | `ConfigMarshaler` (**required**: `Marshal(map[string]string) (string, error)`) and `ConfigUnmarshaler` (**optional**: `Unmarshal(string) (map[string]string, error)`), plus `GetFormat` |
| `errors.go` | `ErrNoFormat`, `UnsupportedParseError` (a parse against an emit-only format) and the format-naming wrappers used by the adapters |
| `generator.go` | `ConfigGenerator` (single fixed format: `Generate`/`Parse`/`GenerateFiles`) and `MultiFormatConfigGenerator` (file-name→adapter dispatch: `RegisterFormat`, `RegisterDefaultFormats`, `Generate`, `Parse`, `GenerateFiles`) |
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

The registered string is matched as a **file-name suffix**, so a whole file name
(`RegisterFormat("server.properties", ...)`) is legal. When several registrations match, the
**longest** wins — selection must not depend on Go's map iteration order, or the same file renders
differently between reconciles and the ConfigMap churns. A file matching nothing falls back to the
properties adapter.

## Emit vs. Parse

The contract is split, and only the emit half is required:

| Interface | Required? | Method |
|---|---|---|
| `ConfigMarshaler` | yes — this is what `RegisterFormat`/`NewConfigGenerator` take | `Marshal` |
| `ConfigUnmarshaler` | no — discovered at runtime by the `Parse` paths | `Unmarshal` |

The framework's write path never reads a generated file back, so a product needing only to *emit* a
format implements `Marshal` alone. `ConfigGenerator.Parse` and `MultiFormatConfigGenerator.Parse`
upgrade the registered adapter to `ConfigUnmarshaler`; when it does not implement it they return
`*UnsupportedParseError`, which names the format (registered extension plus the adapter's Go type)
and the file. Every shipped adapter implements both, asserted at compile time in `format.go`.

Errors from either path name the file and the format: emit failures read
`failed to generate config file "x.env" with the .env (*config.EnvAdapter) format: …`, and
`ConfigMapBuilder.WithMergedConfig` wraps that with the ConfigMap's namespace/name.

## Adapter Contract

Adapters validate their input and return an error rather than emitting output the target parser
would misread:

- **YAML** only round-trips a *flat* mapping. `Unmarshal` rejects a nested value, a sequence or
  scalar document root, complex keys, and a duplicate key (invalid YAML — silently keeping one of
  the two would return a value the document does not carry). `Marshal` quotes values that would
  otherwise change type (`true`, `123`) and emits multi-line values as block scalars; keys are
  sorted.
- **Env** rejects a key that is not a shell variable name (`^[A-Za-z_][A-Za-z0-9_]*$`). A value is
  written bare only when every character is in the shell-inert allowlist
  (`[A-Za-z0-9_@%+=:,./-]`); anything else — a command separator, a redirection, a subshell, a
  tilde, whitespace — is double-quoted with `$`, backticks, `\` and `"` escaped, so sourcing the
  file can never run a config value. `\n`, `\r` and `\t` in values are dotenv-style escapes, so a
  multi-line value is *not* byte-faithful when a POSIX shell sources the file. On read, a
  single-quoted value is taken literally, as the shell does.
- **INI** rejects a key or value containing a line break, a key containing `=` or `:`, and a key
  starting with `[`, `#` or `;`.
- **Properties** round-trips exactly. Separators, comment markers and the edge whitespace of a key
  or value are escaped on write and honoured on read, so keys such as `a ` or `#a` come back
  unchanged; a value ending in an escaped backslash does not swallow the following line.
  Whitespace that is *not* escaped is layout (indentation, padding around the separator, the
  indentation of a continuation line) and is dropped, as a hand-written file expects. `\uXXXX`
  escapes are decoded on read, surrogate pairs included.
- **XML** rejects text XML 1.0 cannot carry (C0 control characters other than tab/newline/CR, and
  non-UTF-8 bytes) and writes a carriage return as `&#13;`, since a parser normalizes literal line
  endings in content.

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

1. **Adding a new format:** create a `*_adapter.go` implementing `Marshal` — that alone is a
   complete format — then register it (`RegisterFormat(".ext", NewXAdapter())`), and add it to
   `RegisterDefaultFormats` only if every product should get it. Add `Unmarshal` only when
   something actually reads the format back; an adapter in this package ships both, and the
   compile-time assertions in `format.go` enforce that.
2. **Generating configs:** use `MultiFormatConfigGenerator`; it selects the adapter from the file
   name.
3. **Merging configs:** use `ConfigMerger.Merge` and always check `MergedConfig.PodOverrideErrors`
   before treating the result as the user's intent.
