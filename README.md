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

### How a successful rook injection looks in the operator's logs ?
```
time="2021-01-17T12:12:45Z" level=info msg="Inspecting container 'java-test-adi' for java processes"
time="2021-01-17T12:12:46Z" level=info msg="Java processes: [7]"
time="2021-01-17T12:12:46Z" level=info msg="container: java-test-adi, java processes: [7]"
time="2021-01-17T12:12:46Z" level=info msg="Copying the content of '/var/rookout' directory to 'default/java-test-adi-f97bfbf5-pqk52/java-test-adi:/rookout'"
time="2021-01-17T12:12:46Z" level=info msg="Creating 'default/java-test-adi-f97bfbf5-pqk52/java-test-adi:/rookout' if not exists."
time="2021-01-17T12:12:46Z" level=info msg="Copying the content of '/var/rookout' directory to 'default/java-test-adi-f97bfbf5-pqk52/java-test-adi:/rookout' finished"
time="2021-01-17T12:12:50Z" level=info msg="[Rookout] Testing connection to controller\n[Rookout] Rookout version: 0.1.153\n[Rookout] Injecting Java Agent to process id - 7\n[Rookout] Injected successfully\n"
```


## Code structure
- Project's initial structure created by `operator-sdk init`
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


