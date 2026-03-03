# Operator-go

English | [简体中文](./README_zh.md)

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/zncdatadev/operator-go)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/zncdatadev/operator-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/zncdatadev/operator-go/test.yml)](https://github.com/zncdatadev/operator-go/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zncdatadev/operator-go)](https://goreportcard.com/report/github.com/zncdatadev/operator-go)
[![GitHub License](https://img.shields.io/github/license/zncdatadev/commons-operator)](https://github.com/zncdatadev/operator-go/blob/main/LICENSE)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/zncdatadev/operator-go)](https://github.com/zncdatadev/operator-go/releases)


## 功能特性

- Database Connection CRD：为应用程序提供数据库连接和数据库配置，已实现 Mysql、Postgres、Redis
- S3 Connection CRD：为应用程序提供 S3 连接和 S3 存储桶配置
- Authentication CRD：为应用程序提供灵活的身份认证，已配置 oidc
- 错误和条件常量
- Kubernetes 对象创建或更新优化
