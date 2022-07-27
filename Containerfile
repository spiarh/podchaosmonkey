FROM docker.io/golang:1.18 as builder
WORKDIR /workspace
COPY ./ ./
RUN make build

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/podchaosmonkey /podchaosmonkey
ENTRYPOINT ["/podchaosmonkey"]
CMD ["-v=3"]
