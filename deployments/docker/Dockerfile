# Base build image
FROM golang:1.11-alpine AS build_base
ARG SVC_NAME

# Install some dependencies needed to build the project
RUN apk add make protobuf git gcc g++ libc-dev
WORKDIR /go/src/github.com/cage1016/gokitconsulk8s

# Force the go compiler to use modules
ENV GO111MODULE=on

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

#This is the ‘magic’ step that will download all the dependencies that are specified in
# the go.mod and go.sum file.
# Because of how the layer caching system works in Docker, the  go mod download
# command will _ only_ be re-run when the go.mod or go.sum file change
# (or when we add another docker instruction this line)
RUN go mod download

ENV GOLANG_PROTOBUF_VERSION=1.3.0
RUN GO111MODULE=on go get github.com/golang/protobuf/protoc-gen-go@v${GOLANG_PROTOBUF_VERSION}
RUN cp /go/bin/protoc-gen-go /usr/local/bin/

ENV GOGO_PROTOBUF_VERSION=1.2.1
RUN GO111MODULE=on go get \
  github.com/gogo/protobuf/protoc-gen-gofast@v${GOGO_PROTOBUF_VERSION}
RUN cp /go/bin/protoc-gen-gofast /usr/local/bin/

RUN echo $SVC_NAME

ARG SVC_NAME
# Here we copy the rest of the source code
COPY . .

RUN make $SVC_NAME \
    && mv build/gokitconsulk8s-$SVC_NAME /exe

ENTRYPOINT ["/exe"]
