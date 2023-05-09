#!/bin/sh

INNER_VERSION="$(git describe --tags --abbrev=0)-master"
EXTERNAL_VERSION=$(python version_utils.py read)

## Publishing operator & init container images

# Pulling image from rookout's bucket
docker pull us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator:${INNER_VERSION}
docker pull us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-ubi:${INNER_VERSION}
docker pull us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container:${INNER_VERSION}
docker pull us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container-ubi:${INNER_VERSION}

# Tagging image with dockerhub name and right version
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator:${INNER_VERSION} rookout/k8s-operator:${EXTERNAL_VERSION}
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-ubi:${INNER_VERSION} rookout/k8s-operator-ubi:${EXTERNAL_VERSION}
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator:${INNER_VERSION} rookout/k8s-operator:latest
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-ubi:${INNER_VERSION} rookout/k8s-operator-ubi:latest
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container:${INNER_VERSION} rookout/k8s-operator-init-container:${EXTERNAL_VERSION}
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container-ubi:${INNER_VERSION} rookout/k8s-operator-init-container-ubi:${EXTERNAL_VERSION}
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container:${INNER_VERSION} rookout/k8s-operator-init-container:latest
docker tag us-central1-docker.pkg.dev/rookoutdevelopment/development-images/rookout-k8s-operator-init-container-ubi:${INNER_VERSION} rookout/k8s-operator-init-container-ubi:latest

# Logging into dockerhub and pushing
docker login -u ${DOCKERHUB_USERNAME} -p ${DOCKERHUB_PASSWORD}
docker push rookout/k8s-operator:${EXTERNAL_VERSION}
docker push rookout/k8s-operator-ubi:${EXTERNAL_VERSION}
docker push rookout/k8s-operator:latest
docker push rookout/k8s-operator-ubi:latest
docker push rookout/k8s-operator-init-container:${EXTERNAL_VERSION}
docker push rookout/k8s-operator-init-container-ubi:${EXTERNAL_VERSION}
docker push rookout/k8s-operator-init-container:latest
docker push rookout/k8s-operator-init-container-ubi:latest
