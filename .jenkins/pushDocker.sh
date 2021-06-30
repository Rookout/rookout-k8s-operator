#!/bin/sh
set -ex

cd ../;

echo "Pushing Operator Init Container Docker..."
make push_init_container INNER_VERSION=${NEW_VERSION}

echo "Pushing Operator Docker..."
make docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}