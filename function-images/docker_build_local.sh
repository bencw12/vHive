#!/bin/bash

# MIT License
# 
# Copyright (c) 2020 Dmitrii Ustiugov and EASE lab.
# 
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
# 
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
# 
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

set -euo pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

pushd $DIR/python/grpc > /dev/null

docker build --target base \
       --network=host \
       --tag py_grpc:base .

docker build --target builder \
       --network=host \
       --cache-from py_grpc:base \
       --tag py_grpc:builder .

popd > /dev/null

cd $DIR

TAG=var_workload

for wld in $@; do
    wld=`basename $wld`
    pushd $DIR/python/$wld > /dev/null

    docker build --target builder_workload \
	   --network=host \
           --cache-from=py_grpc:base \
           --cache-from=py_grpc:builder \
           --cache-from=$wld:builder_workload \
           --tag $wld:builder_workload . && \
        docker save -o ${wld}_builder.tar $wld:builder_workload

    docker build --target $TAG \
	   --network=host \
           --cache-from=py_grpc:base \
           --cache-from=py_grpc:builder \
           --cache-from=$wld:builder_workload \
           --tag $wld:$TAG . && \
	docker save -o $wld.tar $wld:$TAG
    popd > /dev/null
done
