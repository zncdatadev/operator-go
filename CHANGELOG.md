<!-- markdownlint-disable -->
# CHANGELOG

## v0.12.1 2024-12-12

### refactor

- Improved secret volume scope interface readable (#286)
- Refactored vector build to make it easier to use (#284)
- Used getter methods for labels and annotations in PDB options (#274)
- Removed kubebuilder validation tags to fix CR installation failure (#273)

### dependencies

- Bumped golang from 1.23.2 to 1.23.4 (#283)
- Bumped k8s.io/client-go from 0.31.3 to 0.31.4 (#276)
- Bumped github.com/onsi/gomega from 1.35.1 to 1.36.1 (#275, #269)
- Bumped sigs.k8s.io/controller-runtime from 0.19.1 to 0.19.3 (#272, #267)
- Bumped github.com/onsi/ginkgo/v2 from 2.21.0 to 2.22.0 (#268)

## v0.12.0 2024-11-25

### features

- Added API `CreateDoesNotExist` in client package (#241)

### refactor

- Added option argument to rbac and config in builder (#251)
- Refactored GitHub Action (#244)
- Refactored Makefile to be more standardized (#243)
- Bumped domain to kubedoop.dev (#242)
- Added merge role-group from role support (#239)
- Refactored code base to pass Go lint (#246)
- Merge supports structs and safely handles struct pointers (#247)

### dependencies

- Bumped `k8s.io/kubectl` from 0.31.2 to 0.31.3 (#264)
- Bumped `github.com/stretchr/testify` from 1.9.0 to 1.10.0 (#262)
- Bumped `k8s.io/client-go` from 0.31.2 to 0.31.3 (#263)
- Bumped `k8s.io/api` from 0.31.2 to 0.31.3 (#261)

### bugs

- Fixed the origin cli-args override to empty when `cliOverrides` is nil in builder (#256)
- Fixed `clusterIp` is None when `serviceType` is nodePort in builder (#255)
- Fixed vector config name error (#254)
- Checked container name is passed for log config in productlogging (#253)
- Fixed vector watcher path error (#252)
- Fixed set `gracefulShutdownTimeout` panic in builder (#250)
- Fixed logback render some nil value in productlogging (#240)

### tests

- Updated example data in reconciler tests (#249)

### chore

- Updated shields in README (#245)
- Added code of conduct (#258)
- Changed update interval from weekly to daily (#260)
- Updated project license (#257)

## v0.11.2 2024-11-08

### bugs

- Fixed typo `providerHint` in auth (#234)
- Added the missed `logType` field and corrected typo in productlogging (#231)

### dependencies

- Bumped toolchain in Makefile to latest version (#233)
- Bumped golang from 1.23.0 to 1.23.2 (#232)

### chore

- Refactored GitHub Action (#235)

## v0.11.1 2024-11-06

### bugs

- Fixed nil `RoleConfigSpec` handling in `PodDisruptionBudget` reconciliation

## v0.11.0 2024-11-05

### features

- Added PodDisruptionBudget support and reconciliation logic (#227)
- Added readiness probe for vector (#224)
- Added LoggingSpec support (#210)
- Added AuthenticationSpec support (#209)
- Enhanced rbac builder to add policy rules (#196)

### improvements

- Refactored log config generation in productlogging (#220)
- Replaced slice dereference with ptr.To for replica counts (#223)
- Renamed CommandOverrides to CliOverrides for consistency (#222)
- Unified Options struct to Option type for consistency in builder (#221)
- Changed AuthenticationClass to cluster scope in api (#208)
- Changed authentication.oidc.provisioner to providerHint in api (#207)

### bugs

- Fixed client.Get to reset err and ignore object does not exist error (#219)
- Fixed container env out of order in build (#211)
- Appended vector data dir volume (#199)

### dependencies

- Bumped github.com/onsi/gomega from 1.34.2 to 1.35.1 (#225)
- Bumped github.com/onsi/ginkgo/v2 from 2.20.2 to 2.21.0 (#226)
- Bumped k8s.io/kubectl from 0.31.1 to 0.31.2 (#216)
- Bumped k8s.io/client-go from 0.31.1 to 0.31.2 (#215)
- Bumped sigs.k8s.io/controller-runtime from 0.19.0 to 0.19.1 (#218)

### ci

- Removed doc issue template in GitHub Actions (#206)

## v0.10.0 2024-09-20

### features

- Remove `expirationTime` from constants
- Add `AnnotationSecretsCertRestartBuffer` to constants
- Add inline connection to s3 bucket connection
- Refactor cluster and role reconciler to improve log and error handling, and
improve the cluster stoppped logic
- Add CRD doc for LDAP provider credentials
- Refactor `client.Get`, now it wrapped `ctrlclient.Get` and add `client.GetWithOwnerNamespace` `client.GetWithObject`
- Refactor config builder, remove `AddDecodeData` and `AddDecodeData`. Secret builder use `stringData` now, it don't need to decode data
- Add `AddItem` to config builder to add single item to config
- Add Properties util to config package, support get and set properties, and move xml util to config package.
- Service builder support listener class
- Rename image spec fields, such as `platformVersion` to `kubedoopVersion`

### bugs

- Remove `AddDecodeData` and `AddDecodeData` from config builder, because it only convert string to byte, and it is not necessary

### chore

### dependencies

- bump k8s.io/client-go from 0.31.0 to 0.31.1
- bump k8s.io/kubectl from 0.31.0 to 0.31.1

## v0.9.2 2024-09-08

### features

- Add `listenerclass` type to apis
- Add enrichment and restarter labels to constants
- Replace vector image with product image, now vector command available in product image

### bugs

- Fix image default policy is `Always`, and can not auto referenct CRD image policy

## v0.9.1 2024-09-03

### features

- Add `ServiceType` field to service builder, can build `headless` service
- Add `productlogging` package, and implement logback, log4j, log4j2 configuration
- Add vector builder for easy integration with vector sidecar

### bugs

- Fix re-reconciler with 0 seconds interval when resource not ready
- Fix rbac builder could not set subjects and roleRef
- Fix container builder missing memory request field

### chore

### dependencies

- Bump github.com/onsi/ginkgo/v2 from 2.20.1 to 2.20.2
- Bump github.com/onsi/gomega from 1.34.1 to 1.34.2

## v0.9.0 2024-08-28

**BROKENCHANGE:**

- Bump k8s version to 1.31.0
- Bump golang version to 1.23.0
- Remove `Database` API group

### features

- Add `ServiceType` field to service builder
- Remove include `zncdata` variable
- Remove `Database` API group
- Enchance bash util script, insert `shutdown` to script

### bugs

### chore

- Fix dependabot commit message prefix is not `build`

### dependencies

#### upgrade

- sigs.k8s.io/controller-runtime from 0.18.2 to 0.19.0
- k8s.io/client-go from 0.30.1 to 0.31.0
- k8s.io/api from 0.30.1 to 0.31.0
- k8s.io/apimachinery from 0.30.1 to 0.31.0
- k8s.io/kubectl from 0.30.1 to 0.31.0
- github.com/cisco-open/k8s-objectmatcher from 1.9.0 to 1.10.0
- github.com/onsi/ginkgo/v2 from 2.20.0 to 2.20.1

- golang from 1.22.6 to 1.23.0

## v0.8.7 2024-08-20

### features

- Change stack name from `stack` to `kubedoop`
- Add directory constants for kubedoop system， directories include data, config, logs, kerberso,tls and so on
- Add generic bash script utilities and constants for operator, include some reuseful scripts, vector constants and so on
- Change `Image` struct field `stackVersion` to `platformVersion`, `Repository` to `Repo`

### bugs

### chore

- Add depbot to auto update dependencies
- Add golang lint to Makefile and update gh action use it
- Remove manual dispatch workflow
- Rename image spec fields, such as `stackVersion` to `platformVersion`

### dependencies

#### upgrade

- golang from 1.22.3 to 1.22.6
- github.com/onsi/ginkgo/v2 from 2.19.0 to 2.20.0
- github.com/stretchr/testify from 1.8.4 to 1.9.0


## v0.8.6 2024-08-07

### features

- Added support for logging to standard output and error streams in Vector, including new log sources and transformers.
- Added log collection for log4j and log4j2 XML log files in Vector, introducing corresponding log sources and transformers.
- Added listener and secret constants.
- Added a volume builder for listener and secret.

### bugs

## v0.8.5 2024-07-26

### features

### bugs

- Fix formatting errors that still exist in `vector.yaml`

## v0.8.4 2024-07-26

### features

- `JobBuilder` default implementation `Job` is public now
- Remove `builder.RoleGroupInfo`, because it confuses with `reconciler.RoleGroupInfo`

### bugs

- Fix Can not get image pull policy in workload builder, now you can get `util.Image` object.
- Fix Can not set vaild replicas and the value always be 1
- Fix the cluster is still running when operation be updated `stopped`
- Fix the `vectory.yaml` indent format error

### chore

- `MergeRoleGroupSpec` method can be modified directly on the passed roleGroup object, we did not
  update any code, but add some test cases to ensure it is correct.

## v0.8.3 2024-07-23

### features

- Add listener CRD type to apis

## v0.8.2 2024-07-22

### bugs

- Fix `CreateOrUpdate` method can not handle crd
- Fix xml configurations may not be in the same propretry order after marshal

## v0.8.1 2024-07-11

### features

- Update `EnvsToEnvVars` to `NewEnvVarsFromMap`, because the previous method name was ambiguous
- Update `XMLConfiguration` functions to support add, delete and marshall and construct from xml string
- Extract `CreateOrUpdate` from `client.Client`, so you can use alone

### bugs

- Fix `XMLConfiguration` can not unmarshal xml string to struct
- Fix `XMLConfiguration` can not handle xml string header when unmarshal to marshal
- Fix `XMLConfiguration` marshal xml string contains escape characters, e.g: \n
- Update `GenerateRandomStr` to generate random string with length, letters, numbers and special characters, and add `GenerateSimplePassword`
- Remove the base64 decode of Secret.Data obtained by controller-runtime, because the data is already decoded

### chore

- Remove incorrectly named functions in `util.configuration.go`

## v0.8.0 2024-07-10

### features

**BROKENCHANGE:** Update `reconciler` and `builder` package

- Add vector log builder
- Refactor reconciler and builder, and add test case
- Add image selection
- Remove s3 finalizer constant

### bugs

- Fix code indentation can not handle multiple lines

## v0.7.0 2024-06-27

### features

**BROKENCHANGE:** Update `S3Connection` and `S3Bucket` group to `S3Connection.s3.zncdata.dev` and `S3Bucket.s3.zncdata.dev`, Update `DatabaseConnection` and `Database` group to `DatabaseConnection.database.zncdata.dev` and `Database.database.zncdata.dev`

- Add group `s3.zncdata.dev` to `s3` package, and move `S3Connection` and `S3Bucket` to `s3` package
- Add group `database.zncdata.dev` to `database` package, and move `DatabaseConnection` and `Database` to `database` package
- Add `AuthenticationClass.authentication.zncdata.dev`, and support oidc ldap tls and static
- Use `SecretClass` provide `S3Connection` credential

### chore

- typo fix issue template

## v0.6.0 2024-06-24

### features

- Add `cluster operation` `logging` `pdb` `resource` api to commons
- Add resource builder and basic reconciler, you can implement in operator
- Add s3 client-side to verification tls certificate

### chore

- Add issue template


## v0.5.1 2024-05-23

### features

- Bump go version to 1.22

### chore

- Add go boilerplate to auto generate license header

## v0.5.0 2024-05-21

**BREAKCHANGE** Update github group to `zncdatadev`

### features

- Add `properties` configuration util
- Add `xml` configuration util
- Add code of string intendation, tabs and spaces can be converted to each other
- Add name string generator, use `-` to connect words
- Add StatefulSet check to `CreateOrUpdate`
- Add golang template parse function

## v0.4.0 2024-03-20

### features

- Add base64 util function, support method: `Base64.Encode` and `Base64.Decode`

## v0.3.0 2024-0206

### features

- Update domain to `zncdata.dev`
- Update credential struct name
- Fix `mysql` word typo

## v0.2.1 2024-01-17

### bugs

- remove `DatabaseConnection.spec.provider` enum by CRD validation

## v0.2.0 2024-01-16

### bugs

- fix crds not register to k8s
- update `s3bucket.spec.name` to `bucketName` and `databass.spec.name` to `databaseName`

## v0.1.0 2024-01-11

### features

- Add `DatabaseConnection` and `Database` struct, and implement mysql, postgres, redis.
- Add `S3Conection` and `S3Bucket` struct.
- Add `AuthenticationClass` struct, and implement oidc.
- Add errors and conditions constants
- Add `CreateOrUpdate` for k8s object create or update.
