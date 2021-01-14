#!/bin/sh
img=tmaxcloudck/claim-operator:b5.0.0.5
docker rmi $img
make docker-build docker-push IMG=$img
