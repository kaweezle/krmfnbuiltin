/*
Package extras contains additional utility transformers and generators.

[GitConfigMapGeneratorPlugin] is identical to ConfigMapGeneratorPlugin
but automatically creates two properties when run inside a git repository:

  - repoURL gives the URL of the origin remote.
  - targetRevision gives the current branch.

[ExtendedReplacementTransformerPlugin] is a copy of ReplacementTransformerPlugin
that provides extended target paths into embedded data structures. For instance,
consider the following resource snippet:

	helm:
	  parameters:
	    - name: common.targetRevision
	      # This resource is accessible by traditional transformer
	      value: deploy/citest
	    - name: common.repoURL
	      value: https://github.com/antoinemartin/autocloud.git
	  values: |
	    uninode: true
	    apps:
	      enabled: true
	    common:
	      # This embedded resource is not accessible
	      targetRevision: deploy/citest
	      repoURL: https://github.com/antoinemartin/autocloud.git

In the above, the common.targetRevision property of the yaml embedded in the
spec.source.helm.values property is not accessible with the traditional
ReplacementTransformerPlugin. With the extended transformer, you can target
it with:

	fieldPaths:
	- spec.source.helm.parameters.[name=common.targetRevision].value
	- spec.source.helm.values.!!yaml.common.targetRevision

Note the use of !!yaml to designate the encoding of the embedded structure. The
extended transformer supports the following encodings:

  - YAML
  - JSON
  - TOML
  - INI
  - base64
  - Plain text (with Regexp)
*/
package extras
