# ingress2gateway-operator

The `ingress2gateway-operator` is a Kubernetes operator that automatically translates `Ingress` resources into Gateway API `HTTPRoute` resources. By leveraging the [ingress2gateway](https://github.com/kgateway-dev/ingress2gateway) library, it provides a seamless bridge for migrating from legacy Ingress controllers to modern Gateway API implementations.

## Description

This controller watches for `Ingress` resources in your cluster and reconciles them into equivalent `HTTPRoute` resources. It is particularly useful for environments transitioning to the Gateway API, as it allows users to continue using familiar Ingress definitions while benefiting from the advanced traffic management capabilities of Gateway API providers like kGateway.

## Features

- **Automatic Migration**: Automatically generates `HTTPRoute` objects from `Ingress` definitions.
- **Ingress Class Filtering**: Optionally filter which Ingresses to process based on `ingressClassName` or the `kubernetes.io/ingress.class` annotation.
- **Flexible Gateway Target**: Configure a default parent Gateway for all routes, or override it on a per-Ingress basis using annotations.
- **Native Lifecycle Management**: Uses Kubernetes OwnerReferences to ensure that `HTTPRoute` objects are automatically cleaned up when the source `Ingress` is deleted.
- **Deduplication**: Inherits robust translation logic from `ingress2gateway`, including handling of duplicate paths and hosts.

## Configuration

### Controller Flags

The operator can be configured using the following command-line flags:

- `--default-parent-gateway`: (Optional) The default parent Gateway for generated `HTTPRoutes`. Format can be `name` (same namespace as Ingress) or `namespace/name`.
- `--ingress-class`: (Optional) If specified, the controller will only process Ingresses that match this class via `spec.ingressClassName` or the `kubernetes.io/ingress.class` annotation.

### Annotations

You can control the behavior of specific Ingresses using annotations:

- `ingress2gateway.viu.ca/parent-gateway`: Overrides the `--default-parent-gateway` flag for this specific Ingress. Format: `name` or `namespace/name`.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes cluster with Gateway API CRDs installed.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/ingress2gateway-operator:tag
```

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/ingress2gateway-operator:tag
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/ingress2gateway-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/ingress2gateway-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

## Contributing

We welcome contributions! Please feel free to submit Pull Requests or open issues on the project repository.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

