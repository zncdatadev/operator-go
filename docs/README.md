# 文档导航

此目录维护统一的架构、安全规范和 CRD 示例。

- [架构设计](architecture.md#framework-design)：产品职责、输入继承、覆盖、装配、平台依赖、生命周期和数据操作；原有 GenericReconciler API 在同文单独标明适用范围。
- [安全设计](security.md#framework-authentication)：认证、Secret/CSI 与独立数据操作授权。
- [Trino 开发示例](../examples/trino-operator/README.md)：产品配置、原生消费和实际接入方式。
- [交付与验证指南](../hack/framework-e2e/README.md)：生成、构建、部署和可复用验收工具。
- [CRD 示例](examples/)：已有 SDK 的公共结构示例；新框架参考输入见 Trino 示例模块。

讨论、调研、迭代计划、临时原型与运行结果保存在 Git 忽略的 `.local/engineering-notes/`。
它们用于工作上下文，不是正式文档的前置依赖，也不应加入版本库。
