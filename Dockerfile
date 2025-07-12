# -*- mode: dockerfile -*-
#
# A multi-stage Dockerfile that builds a Linux target then creates a small
# final image for deployment.

#
# STAGE 1
#
# Build from source.
#

FROM golang:alpine AS builder

COPY ./ ./

RUN go version
RUN pwd

RUN go build -o passages-signup .

RUN du -sh passages-signup

#
# STAGE 2
#
# Use a tiny base image (Distroless) and copy in the release target. This
# produces a very small output image for deployment.
#

FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /go/passages-signup /

ENV PORT=8082
ENTRYPOINT ["/passages-signup"]
