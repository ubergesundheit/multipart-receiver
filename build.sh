#!/bin/sh

BIN=multipart-receiver

FLAGS=-ldflags="-s -w"

go build -o "$BIN" "$FLAGS" main.go
