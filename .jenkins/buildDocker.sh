#!/bin/sh
set -ex
cd ../;
echo "Building Operator Init Container Docker..."
make build_init_container INNER_VERSION=${NEW_VERSION}

echo "Building Operator Docker..."
make docker-build IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}