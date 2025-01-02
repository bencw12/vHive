FROM python:3.8.1-alpine3.11

ENV GRPC_PYTHON_VERSION=1.26.0
RUN apk update && \
    apk add --update --no-cache gcc g++ protobuf && \
    pip3 install --user protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}

RUN cp -r /root/.local/lib/* /lib/

RUN apk add --no-cache \
    g++ gcc gfortran file binutils linux-headers \
    musl-dev python3-dev cython openblas-dev lapack-dev \
    libstdc++ openblas lapack py3-scipy

COPY requirements.txt .
RUN python -m pip install --upgrade pip
RUN pip3 install --user -r requirements.txt

COPY *.py /
COPY dataset.csv /
COPY lr_model.pk /

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/bin/python", "/server.py"]