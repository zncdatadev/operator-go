# Operator-go

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/zncdatadev/operator-go)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/zncdatadev/operator-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/zncdatadev/operator-go/test.yml)](https://github.com/zncdatadev/operator-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zncdatadev/operator-go)](https://goreportcard.com/report/github.com/zncdatadev/operator-go)
[![GitHub License](https://img.shields.io/github/license/zncdatadev/commons-operator)](https://github.com/zncdatadev/operator-go/blob/main/LICENSE)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/releases)


## Features

- Database Connection CRD, which provides database connection and database configuration for applications, has implemented Mysql, Postgres, Redis
- S3 Connection CRD: Provides S3 connection and S3 bucket configuration for applications
- Authentication CRD provides flexible authentication for applications and has been configured with oidc
- Error and condition constants
- k8s object creation or update optimization
