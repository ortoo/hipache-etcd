#!/bin/sh

mkdir -p ./bin
GOARCH=amd64 GOOS=linux GOBIN=./bin go install