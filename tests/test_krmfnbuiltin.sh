#!/bin/bash

# DEPENDENCEIS
# kustomize
# yq

#set -uexo pipefail
set -e pipefail

trap "rm -rf {patch,replacement}/applications" EXIT


for d in patch replacement; do
    echo "Running Test in $d..."
    cd $d
    rm -rf appllications
    cp -r original applications
    kustomize fn run --enable-exec --fn-path functions applications
    diff <(yq eval -P expected/argocd.yaml) <(yq eval -P applications/argocd.yaml)
    cd ..
done
echo "Done ok 🎉"
