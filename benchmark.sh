#!/usr/bin/env bash

docker run --rm -v=$(pwd):/app -w=/app --cpus=1.5 -m=1000m golang:1.19 go test -bench . -benchmem -benchtime=10s
