#!/bin/sh
img=tmaxcloudck/claim-operator:b5.0.2.8
docker rmi $img
make docker-build docker-push IMG=$img
