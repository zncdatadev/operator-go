# Example operators

Parent: [../AGENTS.md](../AGENTS.md).

`trino-operator/` is a separate Go module with a local SDK replace. Its real executable uses the formal `pkg/framework` API: product C/S/F and Definition, generated input/CRD/registration, then public operator registration on a controller-runtime manager. See its [AGENTS.md](trino-operator/AGENTS.md) and [README.md](trino-operator/README.md).

Examples should demonstrate the public author path and complete deployable manifests. Product generation stays pure, external reads use FactsReader, and SDK pipeline/controller internals are not imported. Generated input artifacts and companions are checked by `make verify-generate`. Test-only runtime tools must remain separate from the product executable and must not be described as product query validation.
