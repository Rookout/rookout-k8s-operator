#!/bin/sh
set -ex

echo "Pushing Operator Init Container Docker..."
cd ../; make push_init_container INNER_VERSION=${NEW_VERSION}

echo "Pushing Operator Docker..."
cd ../; make docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}