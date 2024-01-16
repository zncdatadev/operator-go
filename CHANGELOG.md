# CHANGELOG

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
