<!-- markdownlint-disable -->
# CHANGELOG

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
