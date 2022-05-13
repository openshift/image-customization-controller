FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.11 AS builder
WORKDIR /go/src/github.com/openshift/image-customization-controller
COPY . .
RUN CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o bin/image-customization-controller cmd/controller/main.go
RUN CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o bin/image-customization-server cmd/static-server/main.go

FROM registry.ci.openshift.org/ocp/4.11:base
COPY --from=builder /go/src/github.com/openshift/image-customization-controller/bin/image-customization-controller /
COPY --from=builder /go/src/github.com/openshift/image-customization-controller/bin/image-customization-server /

# Binarys have been renamed to machine-image-customization-*, using a symlink for
# now to ensure backward compat, can be removed at a later stage
RUN ln -s /image-customization-controller /machine-image-customization-controller
RUN ln -s /image-customization-server /machine-image-customization-server

RUN dnf install -y nmstate
