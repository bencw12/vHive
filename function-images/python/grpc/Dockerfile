FROM python:3.8.1-alpine3.11 AS base

FROM base AS builder
ENV GRPC_PYTHON_VERSION=1.26.0
RUN apk update && \
    apk add --update --no-cache gcc g++ protobuf && \
    pip3 install --user protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}
    # tools are not required for running
    #grpcio-tools==${GRPC_PYTHON_VERSION}
