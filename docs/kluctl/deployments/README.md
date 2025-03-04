<!-- This comment is uncommented when auto-synced to www-kluctl.io

---
title: "Deployments"
linkTitle: "Deployments"
weight: 2
description: >
    Deployments and sub-deployments.
---
-->

## Table of Contents

1. [Deployments](./deployment-yml.md)
2. [Kustomize Integration](./kustomize.md)
3. [Container Images](./images.md)
4. [Helm Integration](./helm.md)
5. [OCI Integration](./oci.md)
6. [SOPS Integration](./sops.md)
7. [Hooks](./hooks.md)
8. [Readiness](./readiness.md)
9. [Tags](./tags.md)
10. [Annotations](./annotations)

A deployment project is collection of deployment items and sub-deployments. Deployment items are usually
[Kustomize](./kustomize.md) deployments, but can also integrate [Helm Charts](./helm.md).

## Basic structure

The following visualization shows the basic structure of a deployment project. The entry point of every deployment
project is the `deployment.yaml` file, which then includes further sub-deployments and kustomize deployments. It also
provides some additional configuration required for multiple kluctl features to work as expected.

As can be seen, sub-deployments can include other sub-deployments, allowing you to structure the deployment project
as you need.

Each level in this structure recursively adds [tags](./tags.md) to each deployed resources, allowing you to control
precisely what is deployed in the future.

Some visualized files/directories have links attached, follow them to get more information.

<!-- markdown-link-check-disable -->
<pre>
-- project-dir/
   |-- <a href="./deployment-yml">deployment.yaml</a>
   |-- .gitignore
   |-- kustomize-deployment1/
   |   |-- kustomization.yaml
   |   `-- resource.yaml
   |-- sub-deployment/
   |   |-- deployment.yaml
   |   |-- kustomize-deployment2/
   |   |   |-- resource1.yaml
   |   |   `-- ...
   |   |-- kustomize-deployment3/
   |   |   |-- kustomization.yaml
   |   |   |-- resource1.yaml
   |   |   |-- resource2.yaml
   |   |   |-- patch1.yaml
   |   |   `-- ...
   |   |-- <a href="./helm">kustomize-with-helm-deployment/</a>
   |   |   |-- charts/
   |   |   |   `-- ...
   |   |   |-- kustomization.yaml
   |   |   |-- helm-chart.yaml
   |   |   `-- helm-values.yaml
   |   `-- subsub-deployment/
   |       |-- deployment.yaml
   |       |-- ... kustomize deployments
   |       `-- ... subsubsub deployments
   `-- sub-deployment/
       `-- ...
</pre>
<!-- markdown-link-check-enable -->

## Order of deployments
Deployments are done in parallel, meaning that there are usually no order guarantees. The only way to somehow control
order, is by placing [barriers](./deployment-yml.md#barriers) between kustomize deployments.
You should however not overuse barriers, as they negatively impact the speed of kluctl.

## Plain Kustomize

It's also possible to use Kluctl on plain Kustomize deployments. Simply run `kluctl deploy` from inside the
folder of your `kustomization.yaml`. If you also don't have a `.kluctl.yaml`, you can also work without targets.

Please note that pruning and deletion is not supported in this mode.