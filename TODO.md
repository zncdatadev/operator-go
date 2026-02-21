# TODO List

This document tracks issues and improvements that need to be addressed in future iterations.

---

## Security Documentation

### High Priority
- [ ] Add threat model description to security documentation
- [ ] Define security boundaries clearly
- [ ] Add audit logging specifications
- [ ] Document secret rotation strategies
- [ ] Add intrusion detection/response procedures
- [ ] Document security version update strategy

### Medium Priority
- [ ] Provide actual RBAC rule examples (not just conceptual descriptions)
- [ ] Document minimum privilege set for common scenarios
- [ ] Address privilege escalation risks

---

## Orphaned Resource Cleanup

### Implementation Improvements
- [ ] Implement "soft delete" or "delayed delete" protection mechanism
- [ ] Add pre-deletion backup/confirmation mechanism for critical resources
- [ ] Document deletion rollback procedures

---

## Webhook Integration

### Error Handling
- [ ] Define degradation strategy when Webhook is unavailable
- [ ] Document Webhook timeout configurations
- [ ] Handle edge case: MutatingWebhook fails but ValidatingWebhook passes
- [ ] Add Webhook health check and automatic recovery

---

## Error Handling Strategy

### Classification and Handling
- [ ] Define clear criteria for which errors trigger Fail-Fast vs. continue
- [ ] Document error classification standards
- [ ] Add error recovery/retry strategies for different error types
- [ ] Implement partial failure recovery mechanisms for Extensions

---

## Performance and Scalability

### Benchmarks and Guidelines
- [ ] Define maximum number of CRs a single Operator can manage
- [ ] Document reconcile loop performance benchmarks
- [ ] Provide tuning recommendations for large-scale clusters
- [ ] Document resource limits and quota recommendations
- [ ] Add performance testing framework

---

## Health Check

### Security Considerations
- [ ] Address security risks of ExecUtil executing commands inside containers
- [ ] Implement command whitelist or validation mechanism
- [ ] Add audit logging for health check commands

---

## Future Enhancements (from architecture.md)

- [ ] Support ConversionWebhook for smooth CRD version upgrades
- [ ] Extend extension point fault tolerance with degradation strategies
- [ ] Add monitoring metrics for extension execution time and resource cleanup counts
- [ ] Support graceful/gray deletion of role group resources

---

## Completed

- [x] Update Kubernetes version requirement to 1.31+
- [x] Unify zookeeper-related terminology to `zookeeperConfigMap`

---

## Changelog

| Date | Change |
|------|--------|
| 2025-02-21 | Initial TODO list created from documentation review |
