FROM cgr.dev/chainguard/static

WORKDIR /

# Copy the pre-built test binary based on target platform
ARG TARGETARCH
COPY bin/e2e-tests-${TARGETARCH} /e2e-tests

# Run the compiled test binary
ENTRYPOINT ["/e2e-tests"]
