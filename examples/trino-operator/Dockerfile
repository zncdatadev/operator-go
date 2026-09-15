# Build context is bin/image, populated by `make docker-build` from this checkout.
# The operator uses the explicit Kubernetes CA from in-cluster or REST configuration.
FROM scratch
COPY manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
