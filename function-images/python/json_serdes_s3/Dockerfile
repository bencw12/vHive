FROM python:3.8.1-alpine3.11 AS base

ENV GRPC_PYTHON_VERSION=1.26.0
RUN apk update && \
    apk add --update --no-cache gcc g++ protobuf libffi-dev && \
    pip3 install --user protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}

RUN cp -r /root/.local/lib/* /lib/
COPY requirements.txt .
RUN pip3 install --user -r requirements.txt

COPY *.py /
RUN apk add libstdc++ --update --no-cache

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python", "/server.py"]