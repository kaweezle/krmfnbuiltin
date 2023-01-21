# krmfnbuiltin

[![stability-beta](https://img.shields.io/badge/stability-beta-33bbff.svg)](https://github.com/mkenney/software-guides/blob/master/STABILITY-BADGES.md#beta)

krmfnbuiltin is a
[kustomize plugin](https://kubectl.docs.kubernetes.io/guides/extending_kustomize/)
that you can use to perform in place transformation in your kustomize projects.

## Rationale

`kustomize fn run` allows performing _in place_ transformation of KRM
(kubernetes Resource Model) resources. This is handy to perform modification
operations on GitOps repositories (see the [functions tutorial]). Unfortunately,
the builtin transformers are not available to `kustomize fn run`, as it expects
a `container` or `exec` annotation in the transformer resource pointing to a krm
function docker image or executable.

`krmfnbuiltin` provides both the image and executable allowing the use of any
builtin transformer.

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
    #    image: ghcr.io/kaweezle/krmfnbuiltin:v0.0.1
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

You now can commit the 10 modified manifests in your branch.

<!-- prettier-ignore-start -->

[kpt]: https://kpt.dev/guides/rationale
[functions tutorial]: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/tutorials/function-basics.md

<!-- prettier-ignore-end -->
