<!-- markdownlint-disable -->
# CHANGELOG

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
