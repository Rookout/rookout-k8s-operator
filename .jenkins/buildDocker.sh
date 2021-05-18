#!/bin/sh
set -ex

echo "Building Operator Docker..."

cd ../; make docker-build IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}