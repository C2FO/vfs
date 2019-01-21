#!/usr/bin/env bash

#set -e  // this will cause failure if grep doesn't match anything
echo "mode: atomic" > coverage.txt

for d in $(go list ./... | grep -v vendor); do
    go test -race -coverprofile=profile.out -covermode=atomic $d
    if [ -f profile.out ]; then
        grep -v 'mode: atomic' profile.out >> coverage.txt
        rm profile.out
    fi
done
