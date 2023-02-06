# krmfnbuiltin

[![stability-beta](https://img.shields.io/badge/stability-beta-33bbff.svg)](https://github.com/mkenney/software-guides/blob/master/STABILITY-BADGES.md#beta)

krmfnbuiltin is a
[kustomize plugin](https://kubectl.docs.kubernetes.io/guides/extending_kustomize/)
providing a set of [KRM Functions] that you can use to perform in place
transformation in your kustomize projects.

<!-- markdownlint-disable MD033 -->

<!-- TABLE OF CONTENTS -->
<details open="true">
  <summary>Table of Contents</summary>
  <ol>
    <li><a href="#rationale">Rationale</a></li>
    <li><a href="#usage-example">Usage Example</a></li>
    <li><a href="#use-of-generators">Use of generators</a></li>
    <li><a href="#keeping-or-deleting-generated-resources">Keeping or deleting generated resources</a></li>
    <li><a href="#extensions">Extensions</a>
        <ul>
            <li><a href="#remove-transformer">Remove Transformer</a></li>
            <li><a href="#configmap-generator-with-git-properties">ConfigMap generator with git properties</a></li>
            <li><a href="#heredoc-generator">Heredoc generator</a></li>
            <li><a href="#kustomization-generator">Kustomization generator</a></li>
            <li><a href="#sops-decryption-generator">Sops decryption generator</a></li>
            <li><a href="#extended-replacement-in-structured-content">Extended replacement in structured content</a>
                <ul><li><a href="#replacements-source-reuse">Replacements source reuse</a></li></ul>
            </li>
        </ul>
    </li>
    <li><a href="#installation">Installation</a></li>
    <li><a href="#argo-cd-integration">Argo CD integration</a></li>
    <li><a href="#related-projects">Related projects</a></li>
  </ol>
</details>
<!-- markdownlint-enable MD033 -->

## Rationale

`kustomize fn run` allows performing _in place_ transformation of KRM
(Kubernetes Resource Model) resources. This is handy to perform structured
modification operations on GitOps repositories (aka _shift left_, see the
[functions tutorial] and the [KRM Functions Specification][krm functions]).
Unfortunately, the builtin transformers are not available to `kustomize fn run`,
as it expects the function to be contained in an external `container` or
`exec`utable .

`krmfnbuiltin` provides both the image and executable allowing the use of any
kustomize builtin transformer or generator, along with some additional goodies.

## Usage Example

Let's imagine that you have a GitOps repository containing **10** Argo CD
applications in the `applications` folder. The following is the manifest for one
of them:

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
another cluster. You obtain a new repository,
`https://github.com/myname/autocloud.git`, on which you create a branch named
`feature/experiment` for development. For the deployment to the development
cluster to use the right repository and branch, you need to change `repoURL` and
`targetRevision` for all the applications. You can do that by hand, but this is
cumbersome and **error prone**.

On a Kustomization, you would have done:

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
to modify the actual application manifests on your branch. This is where KRM
functions shine. To do that, you can write a function file in a `functions`
directory:

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
    #    image: ghcr.io/kaweezle/krmfnbuiltin:v0.4.0
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

And then you can apply your modification with the following command:

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

`krmfnbuiltin` provides all the Kustomize
[builtin generators](https://kubectl.docs.kubernetes.io/references/kustomize/builtins/).

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
    # This annotation will be transferred to the generated ConfigMap
    config.kaweezle.com/local-config: "true"
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
    # Put this annotation in the last transformation to remove the generated resource
    config.kaweezle.com/prune-local: "true"
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

- ✔️ The actual values (repo url and revision) are only specified once in the
  config map generator.
- ✔️ `spec.source.helm.parameters.[name=common.repoURL].value` is a path more
  specific than `/spec/source/helm/parameters/1/value`. The transformation would
  survive reordering.
- ✔️ The functions file names are prefixed with a number prefix (`01_`, `02_`)
  in order to ensure that the functions are executed in the right order. Note
  that you can group the two functions in one file separated by `---` (this
  would make it unusable from [kpt] though).
- ✔️ The generators contains the annotation:

  ```yaml
  config.kaweezle.com/local-config: "true"
  ```

  that is injected in the generated resource.

- ✔️ In the last transformation, we add the following annotation:

  ```yaml
  config.kaweezle.com/prune-local: "true"
  ```

  In order to avoid saving the generated resources. In the presence of this
  annotation, `krmfnbuiltin` will remove all the resource having the
  `config.kaweezle.com/local-config` annotation.

## Keeping or deleting generated resources

As said above, generated resources are saved by default. To prevent that,
adding:

```yaml
config.kaweezle.com/local-config: "true"
```

on the generators and:

```yaml
config.kaweezle.com/prune-local: "true"
```

On the last transformation will remove those resources. In the absence of these
annotations, the generated resources will be saved in a file named
`.krmfnbuiltin.yaml` located in the configuration directory. You may want to add
this file name to your `.gitignore` file in order to avoid committing it.

In some cases however, we want to _inject_ new resources in the configuration.
This can be done by just omitting the `config.kaweezle.com/local-config`
annotation.

The name of the file containing the generated resources can be set with the
following annotations:

- `config.kaweezle.com/path` for the filename. If it contains directories, they
  will be created.
- `config.kaweezle.com/index` For the starting index of the resources in the
  file.

Example:

```yaml
apiVersion: builtin
kind: ConfigMapGenerator
metadata:
  name: configuration-map
  annotations:
    # config.kaweezle.com/local-config: "true"
    config.kaweezle.com/path: local-config.yaml
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
```

With these annotations, the generated config map will be saved in the
`local-config.yaml` file in the configuration directory.

If the file name is empty, i.e. the annotation is:

```yaml
config.kaweezle.com/path: ""
```

The generated resources will be saved each in its own file with the pattern:

```text
<namespace>/<kind>_<name>.yaml
```

For instance:

```text
kube-flannel/daemonset_kube-flannel-ds.yaml
```

## Extensions

This section describes the krmfnbuiltin additions to the Kustomize transformers
and generators as well as the _enhancements_ that have been made to some of
them.

### Remove Transformer

In the case the transformation(s) involves other transformers than
`krmfnbuiltin`, the `config.kaweezle.com/prune-local` may not be available to
remove resources injected in the transformation pipeline. For this use case,
`krmfnbuiltin` provides `RemoveTransformer`:

```yaml
apiVersion: builtin
kind: RemoveTransformer
metadata:
  name: replacement-transformer
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: ../../krmfnbuiltin
targets:
  - annotationSelector: config.kaweezle.com/local-config
```

Each target specified in the `targets` field follows the
[patches target convention](https://kubectl.docs.kubernetes.io/references/kustomize/builtins/#field-name-patches).

Note that you can use the Kustomize recommended method with a
`PatchStrategicMergeTransformer` and a `$patch: delete` field. The above
transformation is however more explicit.

### ConfigMap generator with git properties

`GitConfigMapGenerator` work identically to `ConfigMapGenerator` except it adds
two properties of the current git repository to the generated config map:

- `repoURL` contains the URL or the remote specified by `remoteName`. by
  default, it takes the URL of the remote named `origin`.
- `targetRevision` contains the name of the current branch.

This generator is useful in transformations that use those values, like for
instance Argo CD application customization. Information about the configuration
of the generator can be found in the [ConfigMapGenerator kustomize
documentation].

The following function configuration:

```yaml
# 01_configmap-generator.yaml
apiVersion: builtin
kind: GitConfigMapGenerator
metadata:
  name: configuration-map
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
remoteName: origin # default
```

produces the following config map (comments mine):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: configuration-map
  namespace: argocd
  annotations:
    # add config.kaweezle.com/prune-local: "true" to last transformer to remove
    config.kubernetes.io/local-config: "true"
    # Add .generated.yaml to .gitignore to avoid mistakes
    internal.config.kubernetes.io/path: .generated.yaml
    config.kubernetes.io/path: .generated.yaml
data:
  repoURL: git@github.com:kaweezle/krmfnbuiltin.git
  targetRevision: feature/extended-replacement-transformer
```

### Heredoc generator

We have seen in [Use of generators](#use-of-generators) how to use
`ConfigMapGenerator` to _inject_ values in order to use them in downstream
transformers, replacements in particular. It has however some limitations, due
to the _flat nature_ of ConfigMaps and the fact that values are only strings.
The former makes it difficult to organize replacement variables and the later
prevents structural (_object_) replacement. For object replacements we can use
`PatchStrategicMergeTransformer`, but then we loose the `ReplacementTransformer`
advantage of using the same source for several targets and end up having
duplicate YAML snippets.

`krmfnbuiltin` allows injecting any KRM resource in the transformation by just
adding the `config.kaweezle.com/inject-local: "true"` annotation to the function
configuration. For instance:

```yaml
apiVersion: config.kaweezle.com/v1alpha1
kind: LocalConfiguration
metadata:
  name: traefik-customization
  annotations:
    # This will inject this resource. like a ConfigMapGenerator, but with hierarchical
    # properties
    config.kaweezle.com/inject-local: "true"
    # This annotation will allow pruning at the end
    config.kaweezle.com/local-config: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
data:
  # kustomization
  traefik:
    dashboard_enabled: true
    expose: true
  sish:
    # New properties
    server: target.link
    hostname: myhost.target.link
    host_key: AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAID+4/eqtPTLC18TE8ZP7NeF4ZP68/wnY2d7mhH/KVs79AAAABHNzaDo=
```

When the function configuration contains the `config.kaweezle.com/inject-local`,
annotation, `krmfnbuiltin` bypasses the generation/transformation process for
this function and return the content of the function config _as if_ it had been
generated. The `config.kaweezle.com/inject-local` annotation as well as the
`config.kubernetes.io/function` annotation are removed from the injected
resource.

The resource contents can then be used in the following transformations, in
particular in replacements, and deleted at the end (with
`config.kaweezle.com/local-config` and `config.kaweezle.com/prune-local`) or
even saved (with `config.kaweezle.com/path`). See
[Keeping or deleting generated resources](#keeping-or-deleting-generated-resources))
for more details.

### Kustomization generator

`KustomizationGenerator` is the kustomize equivalent to
`HelmChartInflationGenerator`. It allows generating resources from a
kustomization.

Example:

```yaml
apiVersion: builtin
kind: KustomizationGenerator
metadata:
  name: kustomization-generator
  annotations:
    config.kaweezle.com/path: "uninode.yaml" # file name to save resources
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
kustomizeDirectory: https://github.com/antoinemartin/autocloud.git//packages/uninode?ref=deploy/citest
```

If this function is run with the following command:

```console
> kustomize fn run --enable-exec --fn-path functions applications
```

It will generate a file named `uninode.yaml` containing all the resources of the
built kustomization in the `applications` directory. With:

```yaml
config.kaweezle.com/path: ""
```

One file will be created per resource (see
[Keeping or deleting generated resources](#keeping-or-deleting-generated-resources)).

**IMPORTANT** The current directory `krmfnbuiltin` runs from is the directory in
which the `kustomize run fn` command has been launched, and **not from the
function configuration folder**. Any relative path should take this into
consideration.

### Sops decryption generator

The `SopsGenerator` generates resources from encrypted content. This content can
be the actual function configuration, in [heredoc](#heredoc-generator) style, or
can come from other files.

It is an inclusion of [krmfnsops](https://github.com/kaweezle/krmfnsops). See
its README file for more information.

In the simplest use case, Imagine you have an unencrypted secret that looks like
this :

```yaml
# argocd-secret.yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: argocd-secret
stringData:
  admin.password: $2a$10$xdlX460lf/WbJNZU5bBoROj6U7oKgPbEcBrnXaemA6gsCzrAJtQ3y
  admin.passwordMtime: "2022-08-30T11:26:42Z"
  webhook.github.secret: ZxqGggxGD070l3dx
  dex.github.clientSecret: 7lqt6nasit6kjtvptmy2dzy1dr796orn5xh05ru1
```

If you encrypt it with [sops], you get something like this:

```yaml
# argocd-secret.yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: argocd-secret
stringData:
  admin.password: ENC[AES256_GCM,data:...,type:str]
  admin.passwordMtime: ENC[AES256_GCM,data:...,type:str]
  webhook.github.secret: ENC[AES256_GCM,data:...,type:str]
  dex.github.clientSecret: ENC[AES256_GCM,data:...==,type:str]
sops:
  age:
    - recipient: age166k86d56...
      enc: |
        -----BEGIN AGE ENCRYPTED FILE-----
        ...
        -----END AGE ENCRYPTED FILE-----
  lastmodified: "2023-02-06T11:36:44Z"
  mac: ENC[AES256_GCM,data:...,type:str]
  pgp: []
  encrypted_regex: ^(data|stringData|.*_keys?|admin|adminKey|password)$
  version: 3.7.3
```

If you want this resource to be unencrypted at kustomization build, you can
create the following generator configuration:

```yaml
# argocd-secret-generator.yaml
apiVersion: krmfnbuiltin.kaweezle.com/v1alpha1
kind: SopsGenerator
metadata:
  name: argocd-secret-generator
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
files:
  - argocd-secret.yaml
```

And insert it in the `generators:` section of your `kustomization.yaml` file:

```yaml
generators:
  - argocd-secret-generator.yaml
```

To avoid adding a generator configuration file to your kustomization, you can
directly transform the encrypted secret file into a KRM generator:

```yaml
# argocd-secret.yaml
apiVersion: krmfnbuiltin.kaweezle.com/v1alpha1
kind: SopsGenerator
type: Opaque
metadata:
  name: argocd-secret
  annotations:
    config.kaweezle.com/kind: "Secret"
    config.kaweezle.com/apiVersion: "v1"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
stringData:
  admin.password: ENC[AES256_GCM,data:...,type:str]
  admin.passwordMtime: ENC[AES256_GCM,data:...,type:str]
  webhook.github.secret: ENC[AES256_GCM,data:...,type:str]
  dex.github.clientSecret: ENC[AES256_GCM,data:...==,type:str]
sops:
  age:
    - recipient: age166k86d56...
      enc: |
        -----BEGIN AGE ENCRYPTED FILE-----
        ...
        -----END AGE ENCRYPTED FILE-----
  lastmodified: "2023-02-06T11:36:44Z"
  mac: ENC[AES256_GCM,data:...,type:str]
  pgp: []
  encrypted_regex: ^(data|stringData|.*_keys?|admin|adminKey|password)$
  version: 3.7.3
```

And your `kustomization.yaml` file would look like:

```yaml
generators:
  - argocd-secret.yaml
```

Note the use of the following annotations:

```yaml
config.kaweezle.com/kind: "Secret"
config.kaweezle.com/apiVersion: "v1"
```

In order to have the generated resource with the proper kind and api version.

**WARNING** While this second inclusion method reduces the number of files, it
disables the [sops] Message authentication code (MAC) verification that prevents
file tampering. Use it at your own risk.

### Extended replacement in structured content

The `ReplacementTransformer` provided in `krmfnbuiltin` is _extended_ compared
to the standard one because it allows structured replacements in properties
containing a string representation of some structured content. It currently
supports the following structured formats:

- YAML
- JSON
- TOML
- INI

It also provides helpers for changing content in base64 encoded properties as
well as a simple regexp based replacer for edge cases. The standard
configuration of the transformer can be found in the [replacements kustomize
documentation].

The typical use case for this is when you have an Argo CD application using a
Helm chart as source with some custom values:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: traefik
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  destination:
    namespace: traefik
    server: https://kubernetes.default.svc
  project: default
  source:
    chart: traefik
    repoURL: https://helm.traefik.io/traefik
    targetRevision: "10.19.5"
    helm:
      parameters: []
      values: |-
        ingressClass:
          enabled: true
          isDefaultClass: true
        ingressRoute:
          dashboard:
            enabled: false
        providers:
          kubernetesCRD:
            allowCrossNamespace: true
            allowExternalNameServices: true
          kubernetesIngress:
            allowExternalNameServices: true
            publishedService:
              enabled: true
        logs:
          general:
            level: ERROR
          access:
            enabled: true
        tracing:
          instana: false
        gobalArguments: {}
        # BEWARE: use only for debugging
        additionalArguments:
         - --api.insecure=false
        ports:
          # BEWARE: use only for debugging
          # traefik:
          #   expose: false
          web:
            redirectTo: websecure
          websecure:
            tls:
              enabled: true
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
  ignoreDifferences: []
```

And that you want your KRM function to personalize the values of the Helm chart.
What you would want is having your replacement path _follow inside_ the values
property by specifying:

```yaml
- spec.source.helm.values.<inside>.ingressRoute.dashboard.enabled
```

This is not possible with the standard `ReplacementTransformer`, but this is is
possible with the one provided by `krmfnbuiltin`. Consider the following
function configurations:

```yaml
# fn-traefik-customization.yaml
apiVersion: builtin
kind: LocalConfiguration
metadata:
  name: traefik-customization
  annotations:
    # This will inject this resource. like a ConfigMapGenerator, but with hierarchical
    # properties
    config.kaweezle.com/inject-local: "true"
    config.kaweezle.com/local-config: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
data:
  # kustomization
  traefik:
    dashboard_enabled: true
    expose: true
---
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: replacement-transformer
  annotations:
    # remove LocalConfiguration after
    config.kaweezle.com/prune-local: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
replacements:
  - source:
      kind: LocalConfiguration
      fieldPath: data.traefik.dashboard_enabled
    targets:
      - select:
          kind: Application
          name: traefik
        fieldPaths:
          # !!yaml tells the transformer that the property contains YAML
          - spec.source.helm.values.!!yaml.ingressRoute.dashboard.enabled
  - source:
      kind: LocalConfiguration
      fieldPath: data.traefik.expose
    targets:
      - select:
          kind: Application
          name: traefik
        fieldPaths:
          - spec.source.helm.values.!!yaml.ports.traefik.expose
```

If you apply this to the directory containing the application, you will obtain a
new application:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: traefik
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
  annotations:
    config.kubernetes.io/path: traefik.yaml
    internal.config.kubernetes.io/path: traefik.yaml
spec:
  destination:
    namespace: traefik
    server: https://kubernetes.default.svc
  project: default
  source:
    chart: traefik
    helm:
      parameters: []
      values: |
        ...
        ingressRoute:
          dashboard:
            enabled: true
        ...
        ports:
          # BEWARE: use only for debugging
          # traefik:
          #   expose: false
          web:
            redirectTo: websecure
          websecure:
            tls:
              enabled: true
          traefik:
            expose: true
    repoURL: https://helm.traefik.io/traefik
    targetRevision: "10.19.5"
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
  ignoreDifferences: []
```

As you can see, inside the `values` property, the yaml has been modified.
`ingressRoute.dashboard.enabled` is now `true` and `port.traefik.expose` is also
`true`. Notice that this last property, also present as a comment, has been
inserted at the end of the `ports` section.

Now for a more _extreme_ use case involving regular expressions, imagine you
have the following configuration map defining two files:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: sish-client
  namespace: traefik
  labels:
    app.kubernetes.io/name: "sish-client"
    app.kubernetes.io/component: edge
    app.kubernetes.io/part-of: autocloud
data:
  # ~/.ssh/config file
  config: |
    PubkeyAcceptedKeyTypes +ssh-rsa
    Host sishserver
      HostName holepunch.in
      Port 2222
      BatchMode yes
      IdentityFile ~/.ssh_keys/id_rsa
      IdentitiesOnly yes
      LogLevel ERROR
      ServerAliveInterval 10
      ServerAliveCountMax 2
      RemoteCommand sni-proxy=true
      RemoteForward citest.holepunch.in:443 traefik.traefik.svc:443
  # ~/.ssh/known_hosts with the server key
  known_hosts: |
    [holepunch.in]:2222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID+3abW2y3T5dodnI5O1Z/2KlIdH3bwnbGDvCFf13zlh
```

And imagine you want to modify it to access a different server on another domain
name. You need to change:

- `HostName` in `~/.ssh/config` from `holepunch.in` to the new server address.
- `RemoteForward` in `~/.ssh/config` by changing the address forwarded from
  `citest.holepunch.in` to the new address.
- In `~/.ssh/known_hosts` the name of the host and the key fingerprint of the
  new server.

You can do this by hand, but you may forget something now and the next time.
This is where the regexp transformer comes into play with the following
configuration:

```yaml
apiVersion: builtin
kind: LocalConfiguration
metadata:
  name: configuration-map
  annotations:
    config.kaweezle.com/inject-local: "true"
    config.kaweezle.com/local-config: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
data:
  sish:
    # New properties
    server: target.link
    hostname: myhost.target.link
    host_key: AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAID+4/eqtPTLC18TE8ZP7NeF4ZP68/wnY2d7mhH/KVs79AAAABHNzaDo=
---
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: replacement-transformer
  annotations:
    config.kaweezle.com/prune-local: "true"
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
replacements:
  - source:
      kind: LocalConfiguration
      fieldPath: data.sish.server
    targets:
      - select:
          kind: ConfigMap
          name: sish-client
        fieldPaths:
          - data.config.!!regex.^\s+HostName\s+(\S+)\s*$.1
          - data.known_hosts.!!regex.^\[(\S+)\].1
  - source:
      kind: LocalConfiguration
      fieldPath: data.sish.hostname
    targets:
      - select:
          kind: ConfigMap
          name: sish-client
        fieldPaths:
          - data.config.!!regex.^\s+RemoteForward\s+(\S+):.1
  - source:
      kind: LocalConfiguration
      fieldPath: data.sish.host_key
    targets:
      - select:
          kind: ConfigMap
          name: sish-client
        fieldPaths:
          - data.known_hosts.!!regex.ssh-ed25519\s(\S+).1
```

The _path_ after `!!regex` is composed of two elements. The first one is the
regexp to match. The second one is the the capture group that needs to be
replaced with the source. In the first replacement, the regexp:

```regexp
^\s+HostName\s+(\S+)\s*$
```

can be interpreted as:

> a line starting with one or more spaces followed by `HostName`, then one or
> more spaces and a sequence of non space characters, captured as a group; then
> optional spaces till the end of the line.

The second part of the path, `1`, tells to replace the first capturing group
with the source. With the above, the line:

```sshconfig
      HostName holepunch.in
```

will become

```sshconfig
      HostName target.link
```

#### Replacements source reuse

In the above examples, the `ReplacementTransformer` gets the source data from a
generator that is injected (`config.kaweezle.com/inject-local: "true"`) and then
removed (`config.kaweezle.com/prune-local: "true"`). The extended version of
`ReplacementTransformer` allows specifying a `source:` that can either be a
resource file or the path of a kustomization.

We can create a `properties.yaml` file:

```yaml
# properties.yaml
apiVersion: autocloud.config.kaweezle.com/v1alpha1
kind: PlatformValues
metadata:
  name: autocloud-values
data:
  traefik:
    dashboard_enabled: true
    expose: true
  sish:
    hostname: mydomain.link
    remote: argocd.mydomain.link
    host_key: AAAAC3NzaC1lZDI1NTE5AAAAIEAfLUpTj0fn5sJFW6agmLMsvEacMBvXocyzHLW+AOSQ
    # more configuration below...
```

And then reference it from our replacements:

```yaml
# fn-traefik-customization.yaml
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: replacement-transformer
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: krmfnbuiltin
# Source of replacements
source: properties.yaml
replacements:
  - source:
      kind: LocalConfiguration
      fieldPath: data.traefik.dashboard_enabled
    targets:
      - select:
          kind: Application
          name: traefik
        fieldPaths:
          # !!yaml tells the transformer that the property contains YAML
          - spec.source.helm.values.!!yaml.ingressRoute.dashboard.enabled
```

As the source of the replacement is _sideloaded_, there no need to inject it nor
remove it from the configuration. Also, as the `source` can be a kustomization,
there is no need for it to be local.

## Installation

With each [Release](https://github.com/kaweezle/krmfnbuiltin/releases), we
provide binaries for most platforms as well as Alpine based packages.

On POSIX systems (Linux and Mac), you can install the last version with:

```console
curl -sLS https://raw.githubusercontent.com/kaweezle/krmfnbuiltin/main/get.sh | /bin/sh
```

If you don't want to pipe into shell, you can do:

```console
> KRMFNBUILTIN_VERSION="v0.4.0"
> curl -sLo /usr/local/bin/krmfnbuiltin https://github.com/kaweezle/krmfnbuiltin/releases/download/${KRMFNBUILTIN_VERSION}/krmfnbuiltin_${KRMFNBUILTIN_VERSION}_linux_amd64
```

## Argo CD integration

`krmfnbuiltin` is **NOT** primarily meant to be used inside Argo CD, but instead
to perform _structural_ modifications to the configuration **BEFORE** it's
committed and provided to GitOps.

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

ARG KRMFNBUILTIN_VERSION=v0.4.0

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

krmfnbuiltin works with [kpt]. The `tests/test_krmfnbuiltin_kpt.sh` script
perform the basic tests with kpt.

[knot8] lenses have provided the idea of extended paths.

<!-- prettier-ignore-start -->

[KRM Functions]: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md
[kpt]: https://kpt.dev/guides/rationale
[functions tutorial]: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/tutorials/function-basics.md

[knot8]: https://knot8.io/
[ConfigMapGenerator kustomize documentation]:
  https://kubectl.docs.kubernetes.io/references/kustomize/builtins/#_configmapgenerator_
[replacements kustomize documentation]: https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/replacements/
[sops]: https://github.com/mozilla/sops
<!-- prettier-ignore-end -->
