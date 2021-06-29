#!/bin/sh
set -ex

echo "Building Operator Init Container Docker..."
cd ../; make build_init_container INNER_VERSION=${NEW_VERSION}

echo "Building Operator Docker..."
cd ../; make docker-build IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}