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
    if [ -d expected ]; then
        for f in $(ls -1 applications); do
            echo "  > Checking $f..."
            diff <(yq eval -P expected/$f) <(yq eval -P applications/$f)
        done
    else
        echo "  > No expected result. Skipping check"
    fi
    cd ..
done
echo "Done ok 🎉"
