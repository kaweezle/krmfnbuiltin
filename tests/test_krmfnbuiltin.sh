#!/bin/bash

# DEPENDENCEIS
# sops
# kustomize
# age
# yq

#set -uexo pipefail
set -e pipefail

trap "cp compare/argocd.original.yaml applications/argocd.yaml" EXIT

echo "Running kustomize with transformer..."
kustomize fn run --enable-exec --fn-path functions applications
diff <(yq eval -P compare/argocd.expected.yaml) <(yq eval -P applications/argocd.yaml)
echo "Done ok 🎉"
