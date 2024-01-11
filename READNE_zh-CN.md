# Operator-go

## 1. Operator-go简介

Operator-go 是 zncdata 下的 operator 的公共类库，主要用于处理 operator 的相关逻辑。

## 2. 目录介绍

由于都是公共类库，所以主要代码都放在了pkg目录下。

### pkg/spec 目录

    该目录把zncdata 开发的 operator 的公共部分提取出来，方便应用，后续会陆续增加内容
    其中 commons-spec比较特殊，Commons-spec 是commons-operator 的 spec 部分，commons-operator 在 zncdata 的规划中会被许多
    operator 引用，所以把它单独提取出来，方便其他项目使用。

### pkg/status 目录

    该目录是对 operator 的状态进行定义，目前 zncdata 的 operator 的 status 比较统一，目前抽离了出来，后续会陆续增加内容
