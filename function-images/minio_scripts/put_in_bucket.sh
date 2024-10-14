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

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
SERVERSDIR="$( cd $DIR && cd .. && pwd)"

BUCKET=myminio/mybucket

WORKLOAD=image_rotate_s3
for INPUTFILE in img2.jpeg img3.jpeg
do
    mc cp $SERVERSDIR/$WORKLOAD/$INPUTFILE $BUCKET/$INPUTFILE
done


WORKLOAD=lr_training_s3
for INPUTFILE in dataset.csv dataset2.csv
do
    mc cp $SERVERSDIR/$WORKLOAD/$INPUTFILE $BUCKET/$INPUTFILE
done

WORKLOAD=json_serdes_s3
for INPUTFILE in 1.json 2.json
do
    mc cp $SERVERSDIR/$WORKLOAD/$INPUTFILE $BUCKET/$INPUTFILE
done

WORKLOAD=video_processing_s3
for INPUTFILE in vid1.mp4 vid2.mp4
do
    mc cp $SERVERSDIR/$WORKLOAD/$INPUTFILE $BUCKET/$INPUTFILE
done


