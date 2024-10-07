#!/bin/bash

set -xe

SCRIPT_DIR=$(dirname "$(readlink -f "$0")")
ROOT_DIR="$SCRIPT_DIR/../"
BIN_DIR=$ROOT_DIR/bin/

FC_DIR=$ROOT_DIR/firecracker
FC_BUILD=$FC_DIR/build/cargo_target/x86_64-unknown-linux-musl/release/firecracker
JAILER_BUILD=$FC_DIR/build/cargo_target/x86_64-unknown-linux-musl/release/jailer

FCCTRD_DIR=$ROOT_DIR/firecracker-containerd
SHIM_BUILD=$FCCTRD_DIR/runtime/containerd-shim-aws-firecracker
FCCTRD_BUILD=$FCCTRD_DIR/firecracker-control/cmd/containerd/firecracker-containerd
FCCTR_BUILD=$FCCTRD_DIR/firecracker-control/cmd/containerd/firecracker-ctr


# build firecracker
pushd $FC_DIR

cargo update --dry-run
tools/devtool build --release

popd

cp $FC_BUILD $BIN_DIR
cp $JAILER_BUILD $BIN_DIR

# build firecracker-containerd
pushd $FCCTRD_DIR

git submodule update --init --recursive
make all

popd

cp $SHIM_BUILD $BIN_DIR
cp $FCCTRD_BUILD $BIN_DIR
cp $FCCTR_BUILD $BIN_DIR
