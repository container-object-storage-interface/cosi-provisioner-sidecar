FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="COSI provisioner"

COPY ./bin/cosi-provisioner cosi-provisioner
ENTRYPOINT ["/cosi-provisioner"]
