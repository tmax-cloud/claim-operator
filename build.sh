#!/bin/sh
img=tmaxcloudck/claim-operator:b5.0.4.1
docker rmi $img
make docker-build deploy IMG=$img
