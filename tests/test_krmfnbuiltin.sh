#!/bin/bash

# DEPENDENCEIS
# kustomize
# yq

#set -uexo pipefail
set -e pipefail

trap "find . -type d -name 'applications' -exec rm -rf {} +" EXIT


for d in $(ls -d */); do
    echo "Running Test in $d..."
    cd $d
    rm -rf applications
    cp -r original applications
    echo "  > Performing kustomizations..."
    kustomize fn run --enable-exec --fn-path functions applications
    for f in $(ls -1 expected); do
        echo "  > Checking $f..."
        diff <(yq eval -P expected/$f) <(yq eval -P applications/$f)
    done
    cd ..
done
echo "Done ok 🎉"
