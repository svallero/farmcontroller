#!/bin/sh

IMAGE_NAME=svallero/farmcontroller:v0
# build and push image
#make docker-build-multistage docker-push IMG=${IMAGE_NAME}
# deploy
make deploy IMG=${IMAGE_NAME}
