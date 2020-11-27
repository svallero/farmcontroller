#!/bin/sh

IMAGE_NAME=svallero/farmcontroller:v0
# deploy
make deploy IMG=${IMAGE_NAME}
