// Package framework defines the product-facing contracts of the new operator
// framework. Products declare configuration and runtime intent; input generation,
// merging, resource assembly and reconciliation belong to separate packages.
//
// This package contains domain values and pure value helpers. It does not read
// Kubernetes resources, apply changes or expose the framework's execution plan.
package framework
