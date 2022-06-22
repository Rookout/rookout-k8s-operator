#!/bin/sh
set -ex

cd ../;

echo "Pushing Operator Init Container Docker..."
make push_init_container INNER_VERSION=${NEW_VERSION}

echo "Pushing Operator Docker..."
make docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:${NEW_VERSION}
make docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator-ubi:${NEW_VERSION}


# Auto publishing operator images if the last commit message indicates on external docker version update
if git --no-pager log -1 | grep "Updated Docker external version to"; then
  echo "Auto publishing external images"
  make publish-operator
else
  echo "not publishing to dockerhub"
fi