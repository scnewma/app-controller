# App Controller

This is an example Kubernetes Controller + CRD build using [Kubebuilder](https://kubebuilder.io/). It abstracts a common webapp stack (deployment + service + ingress) into a new `WebApp` CRD.

> Note: This is not production ready.

With this new CRD, a web application can be created in a much more manageable way, like so:

```
apiVersion: webapp.com.shaunnewman/v1alpha1
kind: WebApp
metadata:
  name: webapp-sample
spec:
  instances: 2
  image: gcr.io/kuar-demo/kuard-amd64:blue
  healthCheckEndpoint: /healthy
  cpu: 400m
  memory: 56Mi 
  env:
    TEST: some-value
  routes:
    - webapp-sample.example.com
```

## How to use it

```
git clone https://github.com/scnewma/app-controller.git

# install the CRDs into the cluster
make install

# run locally
make run

# run in-cluster (provide your own registry)
make docker-build docker-push IMG=<registry>/app-controller:latest
make deploy IMG=<registry>/app-controller:latest
```
