# Operator-go

## 1. Introduction to Operator-go

Operator-go is the public class library of operator under zncdata, which is mainly used to handle operator-related logic.

## 2. Directory introduction

Since they are all public class libraries, the main codes are placed in the pkg directory.

### pkg/spec directory

    This directory extracts the public parts of the operator developed by zncdata for easy application, and content will be added in the future.
    Among them, commons-spec is special. Commons-spec is the spec part of commons-operator.
    commons-operator will be referenced by many operators in the planning of zncdata, so it is extracted separately to facilitate the use of other projects.

### pkg/status directory

    This directory defines the status of the operator. Currently, the status of the operator in zncdata is relatively unified.
    It is currently separated and content will be added in the future.
