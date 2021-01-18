# rookout-k8s-operator
Rookout's k8s operator


## Main operator goals

- Install Rookout's SDK on running containers  
- Inject pod metadata into containers to be collected by the SDK

## How to install the operator on a cluster ? 
```
# install the operator
kubectl apply -f ./config/samples/deployment.yaml

# deploy operator's configuration
kubectl apply -f ./config/samples/rookout_v1alpha1_rookout.yaml

# test deployment
kubectl logs deployment.apps/rookout-controller-manager -n rookout -c manager

# uinstall
kubectl delete -f ./config/samples/deployment.yaml
kubectl delete -f ./config/samples/rookout_v1alpha1_rookout.yaml
```

## Code structure
- Operator's entry point : [controllers/rookout_controller.go](./controllers/rookout_controller.go)


## Repo local setup - should be done only once after repo checkout
- Install operator sdk:  `brew install operator-sdk`
- Init repo: `make all`

## Build
`make docker-build docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:1.0`


## References
- Basic operator tutorial - [here](https://sdk.operatorframework.io/docs/building-operators/golang/tutorial/)
- Inspired by [this](https://banzaicloud.com/blog/operator-sdk/) blog post
- Inspired by [prometheus-jmx-exporter-operator](https://github.com/banzaicloud/prometheus-jmx-exporter-operator)
- Controller [watches](https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) - we use them to get notified on pods events


