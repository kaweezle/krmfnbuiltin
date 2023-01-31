#!/bin/bash

# DEPENDENCEIS
# kustomize
# yq

#set -uexo pipefail
set -e pipefail

temp_file=$(mktemp)
temp_file_2=$(mktemp)

trap "find . -type d -name 'applications' -exec rm -rf {} +; rm -f $temp_file $temp_file_2" EXIT

for d in $(ls -d */); do
    echo "Running Test in $d..."
    cd $d
    rm -rf applications
    echo "  > Performing kustomizations..."
    kpt fn source original >$temp_file
    for f in functions/*; do
        cat $temp_file | kpt fn eval - --exec ../../krmfnbuiltin --fn-config $f >$temp_file_2
        mv $temp_file_2 $temp_file
    done
    cat $temp_file | kpt fn sink applications
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
