#!/bin/sh
img=tmaxcloudck/claim-operator:b5.0.4.0
docker rmi $img
make docker-build docker-push IMG=$img
