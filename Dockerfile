# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

FROM golang:1.13-alpine3.11 as go

# Use public go modules proxy
ENV GOPROXY https://proxy.golang.org

WORKDIR /app/prometheus-edge-hub

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -i -o /build/bin/prometheus-edge-hub

FROM alpine:3.11

COPY --from=go /build/bin/prometheus-edge-hub /bin/prometheus-edge-hub

EXPOSE 9091

ENTRYPOINT ["prometheus-edge-hub"]
