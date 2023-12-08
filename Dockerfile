FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.21-openshift-4.16 AS builder
WORKDIR /go/src/github.com/openshift/image-customization-controller
COPY . .
RUN CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o bin/image-customization-controller cmd/controller/main.go
RUN CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o bin/image-customization-server cmd/static-server/main.go

FROM registry.ci.openshift.org/ocp/4.16:base
COPY --from=builder /go/src/github.com/openshift/image-customization-controller/bin/image-customization-controller /
COPY --from=builder /go/src/github.com/openshift/image-customization-controller/bin/image-customization-server /

# Binaries should be renamed to machine-image-customization-*, but to ensure
# backwards compatibility just create symlinks for now.
RUN ln -s /image-customization-controller /machine-image-customization-controller
RUN ln -s /image-customization-server /machine-image-customization-server

RUN dnf install -y nmstate
