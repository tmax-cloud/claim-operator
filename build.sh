#!/bin/sh
img=tmaxcloudck/claim-operator:b5.0.2.3
docker rmi $img
make docker-build docker-push IMG=$img
