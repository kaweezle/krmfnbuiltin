# krmfnbuiltin

[![stability-beta](https://img.shields.io/badge/stability-beta-33bbff.svg)](https://github.com/mkenney/software-guides/blob/master/STABILITY-BADGES.md#beta)

krmfnbuiltin is a
[kustomize plugin](https://kubectl.docs.kubernetes.io/guides/extending_kustomize/)
that you can use to perform in place transformation in your kustomize projects.

<!-- markdownlint-disable MD033 -->

<!-- TABLE OF CONTENTS -->
<details open="true">
  <summary>Table of Contents</summary>
  <ol>
    <li><a href="#rationale">Rationale</a></li>
    <li><a href="#usage-example">Usage Example</a></li>
    <li><a href="#use-of-generators">Use of generators</a></li>
    <li><a href="#installation">Installation</a></li>
    <li><a href="#argo-cd-integration">Argo CD integration</a></li>
    <li><a href="#related-projects">Related projects</a></li>
  </ol>
</details>
<!-- markdownlint-enable MD033 -->

## Rationale

`kustomize fn run` allows performing _in place_ transformation of KRM
(kubernetes Resource Model) resources. This is handy to perform modification
operations on GitOps repositories (see the [functions tutorial]). Unfortunately,
the builtin transformers are not available to `kustomize fn run`, as it expects
a `container` or `exec` annotation in the transformer resource pointing to a krm
function docker image or executable.

`krmfnbuiltin` provides both the image and executable allowing the use of any
builtin transformer or generator.

## Usage Example

Let's imagine that you have a GitOps repository containing in the `applications`
folder a list of **10** Argo CD applications. The following is the manifest for
one of them:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: argo-cd
  namespace: argocd
  annotations:
    autocloud/local-application: "true"
spec:
  destination:
    namespace: argocd
    server: https://kubernetes.default.svc
  ignoreDifferences:
    - group: argoproj.io
      jsonPointers:
        - /status
      kind: Application
  project: default
  source:
    repoURL: https://github.com/kaweezle/autocloud.git
    targetRevision: main
    path: packages/argocd
  syncPolicy:
    automated:
      allowEmpty: true
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

You have 9 other application manifests sharing the same snippet:

```yaml
source:
  repoURL: https://github.com/kaweezle/autocloud.git
  targetRevision: main
```

Let's imagine now that you want to fork this repository for developing on
another cluster. Now you get a new repository,
`https://github.com/myname/autocloud.git`, on which you create a branch named
`feature/experiment` for development. For the deployment to the development
cluster to use the right repository and branch, you need to change `repoURL` and
`targetRevision` for all the applications. You can do that by hand, but this is
**error prone**.

This is where KRM functions shine. on a Kustomization, you would have done:

```yaml
patches:
    - patch: |-
        - op: replace
            path: /spec/source/repoURL
            value: https://github.com/myname/autocloud.git
        - op: replace
            path: /spec/source/targetRevision
            value: feature/experiment
    target:
        group: argoproj.io
        version: v1alpha1
        kind: Application
        # This annotation allow us to identify applications pointing locally
        annotationSelector: "autocloud/local-application=true"
```

But here you don't want to add a new kustomization nesting level. You just want
to modify the actual application manifests on your branch. To do that, you can
write a function:

```yaml
# functions/fn-change-repo-and-branch.yaml
apiVersion: builtin
kind: PatchTransformer
metadata:
  name: fn-change-repo-and-branch
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
    # Can also be:
    #  container:
    #    image: ghcr.io/kaweezle/krmfnbuiltin:v0.0.2
patch: |-
  - op: replace
      path: /spec/source/repoURL
      value: https://github.com/myname/autocloud.git
  - op: replace
      path: /spec/source/targetRevision
      value: feature/experiment
target:
  group: argoproj.io
  version: v1alpha1
  kind: Application
  # This annotation allow us to identify applications pointing locally
  annotationSelector: "autocloud/local-application=true"
```

And then you can apply your modification with the following:

```console
> kustomize fn run --enable-exec --fn-path functions applications
```

**NOTE:** _the `--enable-exec` parameter is not needed if you use the
container._

You obtain the desired modifications in the manifests:

```yaml
source:
  repoURL: https://github.com/mysuer/autocloud.git
  targetRevision: feature/experiment
```

You now can commit the 10 modified manifests in your branch and deploy the
applications.

## Use of generators

Let's imagine that one or more of your applications use an Helm chart that in
turn creates applications. You pass the repo URL and target branch as values to
the Helm Chart with the following:

```yaml
helm:
  parameters:
    - name: common.targetRevision
      value: main
    - name: common.repoURL
      value: https://github.com/kaweezle/autocloud.git
```

For that particular transformation, JSON patches are not practical:

```yaml
patch: |-
  - op: replace
    path: /spec/source/repoURL
    value: https://github.com/antoinemartin/autocloud.git
  - op: replace
    path: /spec/source/targetRevision
    value: deploy/citest
  - op: replace
    path: /spec/source/helm/parameters/1/value
    value: https://github.com/antoinemartin/autocloud.git
  - op: replace
    path: /spec/source/helm/parameters/0/value
    value: deploy/citest
```

You need to hardcode the index of the value to replace in the array, which is
error prone, and you start duplicating values.

It would be better to have a unique _source_ with the right values, and do
`replacements` where needed. You can inject the values with a
`ConfigMapGenerator`

```yaml
# 01_configmap-generator.yaml
apiVersion: builtin
# Use this to inject current git values
# kind: GitConfigMapGenerator
kind: ConfigMapGenerator
metadata:
  name: configuration-map
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
# When using GitConfigMapGenerator, these are automatically injected
literals:
  - repoURL=https://github.com/kaweezle/autocloud.git
  - targetRevision=deploy/citest
```

And then use a `ReplacementTransformer` to inject the values:

```yaml
# 02_replacement-transformer.yaml
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: replacement-transformer
  namespace: argocd
  annotations:
    # Put this annotation in the last transformation to remove generated resources
    config.kubernetes.io/prune-local: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
replacements:
  - source:
      kind: ConfigMap
      fieldPath: data.repoURL
    targets:
      - select:
          kind: Application
          annotationSelector: "autocloud/local=true"
        fieldPaths:
          - spec.source.repoURL
          # This field specification is not related to the index
          - spec.source.helm.parameters.[name=common.repoURL].value
  - source:
      kind: ConfigMap
      fieldPath: data.targetRevision
    targets:
      - select:
          kind: Application
          annotationSelector: "autocloud/local=true"
        fieldPaths:
          - spec.source.targetRevision
          - spec.source.helm.parameters.[name=common.targetRevision].value
```

Some remarks:

- ✔️ The actual values (repo url and revision) are only specified once.
- ✔️ `spec.source.helm.parameters.[name=common.repoURL].value` is path more
  specific than `/spec/source/helm/parameters/1/value`.
- ✔️ The functions file names are prefixed with a number prefix (`01_`, `02_`)
  in order to ensure that the functions are executed in the right order.
- ✔️ In the last transformation, we add the following annotation:

  ```yaml
  config.kubernetes.io/prune-local: "true"
  ```

  In order to avoid saving the generated resources. This is due to an issue in
  kustomize that doesn't filter out resources annotated with
  `config.kubernetes.io/local-config` in the case you are using
  `kustomize fn run` (although it works with `kustomize build`).

As a convenience, for this specific use case, we have added a
`GitConfigMapGenerator` that automatically adds the relevant resources, while
some people may consider this overkill.

## Installation

With each [Release](https://github.com/kaweezle/krmfnbuiltin/releases), we
provide binaries for most platforms as well as Alpine based packages. Typically,
you would install it on linux with the following command:

```console
> KRMFNBUILTIN_VERSION="v0.1.0"
> curl -sLo /usr/local/bin/krmfnbuiltin https://github.com/kaweezle/krmfnbuiltin/releases/download/${KRMFNBUILTIN_VERSION}/krmfnbuiltin_${KRMFNBUILTIN_VERSION}_linux_amd64
```

## Argo CD integration

`krmfnbuiltin` is **NOT** primarily meant to be used inside Argo CD, but instead
to perform _structural_ modifications to the source **BEFORE** the commit.

Anyway, to use `krmfnbuiltin` with Argo CD, you need to:

- Make the `krmfnbuiltin` binary available to the `argo-repo-server` pod.
- Have Argo CD run kustomize with the `--enable-alpha-plugins --enable-exec`
  parameters.

To add krmfnbuiltin on argo-repo-server, the
[Argo CD documentation](https://argo-cd.readthedocs.io/en/stable/operator-manual/custom_tools/)
provides different methods to make custom tools available.

If you get serious about Argo CD, you will probably end up cooking your own
image. This
[docker file](https://github.com/antoinemartin/autocloud/blob/deploy/citest/repo-server/Dockerfile#L45)
shows how to use the above installation instructions in your image. To
summarize:

```Dockerfile
FROM argoproj/argocd:latest

ARG KRMFNBUILTIN_VERSION=v0.1.0

# Switch to root for the ability to perform install
USER root

# Install tools
RUN apt-get update && \
    apt-get install -y curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    curl -sLo /usr/local/bin/krmfnbuiltin https://github.com/kaweezle/krmfnbuiltin/releases/download/${KRMFNBUILTIN_VERSION}/krmfnbuiltin_${KRMFNBUILTIN_VERSION}_linux_amd64

USER argocd
```

You also need to patch the `argo-cm` config map to add the parameters. The
following is a strategic merge patch for it:

```yaml
# argocd-cm.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
data:
  # Options to enable exec plugins (krmfnsops).
  kustomize.buildOptions: "--enable-alpha-plugins --enable-exec"
  ...
```

## Related projects

[kpt], from Google, takes this in place transformation principle to another
level by making resource configuration packages similar to docker images. In
this model, a generator or a transformer along its parameters in much like a
line in a dockerfile. It takes a current configuration as source and generates a
new configuration after transformation.

While it has not been tested, krmfnbuiltin should work with [kpt].

<!-- prettier-ignore-start -->

[kpt]: https://kpt.dev/guides/rationale
[functions tutorial]: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/tutorials/function-basics.md

<!-- prettier-ignore-end -->
