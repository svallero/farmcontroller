#!/bin/sh

IMAGE_NAME=svallero/farmcontroller:nonamespaces

# build image
# multistage build does not work on Mac
make docker-build-multistage IMG=${IMAGE_NAME}
#make docker-build IMG=${IMAGE_NAME}

# push image
#make docker-push IMG=${IMAGE_NAME}
