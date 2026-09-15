# Build the Linux binary with `make materializer-image` from the SDK root.
# The image context contains only that binary; no shell or package manager is needed.
FROM scratch
ARG SOURCE_REVISION=unknown
LABEL org.opencontainers.image.revision=$SOURCE_REVISION \
      dev.kubedoop.framework.materialization-plan=v1 \
      dev.kubedoop.framework.properties-codec=properties-v1
COPY --chmod=0755 materialize /materialize
USER 1001:1001
ENTRYPOINT ["/materialize"]
