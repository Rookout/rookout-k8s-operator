# rookout-k8s-operator
Rookout's k8s operator


## Main operator goals

- Patch deployments with init container that loads rookout agent  
- Inject pod metadata into containers to be collected by the SDK

## Supported Runtimes
- Java (version >= 8) 

## Supported k8s resources
- Deployment (pod resources which not part of deployment not affected by the operator) 

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

### The following log line shows that the operator is ready to patch deployments
```
time="2021-01-20T17:49:10Z" level=info msg="operator configuration updated"
```

### How a successful deployment patch looks in the logs ? 
```
time="2021-01-20T17:49:27Z" level=info msg="adding rookout agent to container <CONTAINER> of deployment <DEPLOYMENT>"
time="2021-01-20T17:49:27Z" level=info msg="deployment <DEPLOYMENT> patched successfully"
```

# Development
## Code structure
- Project's initial structure created by `operator-sdk init`
- Operator's entry point : [/controllers/rookout_controller.go](./controllers/rookout_controller.go)
- Operator Resource API : [/api/v1alpha1/rookout_types.go](./api/v1alpha1/rookout_types.go)

## Repo local setup
- Install operator sdk:  `brew install operator-sdk`
- Init repo: `make all`

## How to run and test on a cluster
- Move to a dev cluster
- Run make build-and-deploy
- It should create an operator on rookout namespace

## Known issues
- Can't inject to PID=1 on linux alpine with openjdk < 14
- Java 7 requires Suns's tools.jar


## References
- Basic operator tutorial - [here](https://sdk.operatorframework.io/docs/building-operators/golang/tutorial/)
- Inspired by [this](https://banzaicloud.com/blog/operator-sdk/) blog post
- Inspired by [prometheus-jmx-exporter-operator](https://github.com/banzaicloud/prometheus-jmx-exporter-operator)
- Controller [watches](https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) - we use them to get notified on pods events
