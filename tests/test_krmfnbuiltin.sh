#!/bin/bash

# DEPENDENCEIS
# kustomize
# yq

#set -uexo pipefail
set -e pipefail

trap "find . -type d -name 'applications' -exec rm -rf {} +" EXIT

export SOPS_AGE_KEY=$(cat - <<EOF
# created: 2023-01-19T19:41:45Z
# public key: age166k86d56ejs2ydvaxv2x3vl3wajny6l52dlkncf2k58vztnlecjs0g5jqq
AGE-SECRET-KEY-15RKTPQCCLWM7EHQ8JEP0TQLUWJAECVP7332M3ZP0RL9R7JT7MZ6SY79V8Q
EOF
)
export SOPS_RECICPIENT="age166k86d56ejs2ydvaxv2x3vl3wajny6l52dlkncf2k58vztnlecjs0g5jqq"


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
