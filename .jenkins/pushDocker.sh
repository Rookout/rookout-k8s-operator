#!/bin/sh
set -ex

echo "Pushing Operator Docker..."

cd ../; make docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}