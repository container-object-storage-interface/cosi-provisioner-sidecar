FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="COSI driver sidecar"

COPY ./bin/cosi-driver cosi-driver
ENTRYPOINT ["/cosi-driver"]
