package main

import (
	"fmt"
	"os"

	"github.com/kaweezle/krmfnbuiltin/pkg/transformers"

	"sigs.k8s.io/kustomize/api/konfig"
	fLdr "sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	// build annotations
	BuildAnnotationPreviousKinds      = konfig.ConfigAnnoDomain + "/previousKinds"
	BuildAnnotationPreviousNames      = konfig.ConfigAnnoDomain + "/previousNames"
	BuildAnnotationPrefixes           = konfig.ConfigAnnoDomain + "/prefixes"
	BuildAnnotationSuffixes           = konfig.ConfigAnnoDomain + "/suffixes"
	BuildAnnotationPreviousNamespaces = konfig.ConfigAnnoDomain + "/previousNamespaces"
	BuildAnnotationsRefBy             = konfig.ConfigAnnoDomain + "/refBy"
	BuildAnnotationsGenBehavior       = konfig.ConfigAnnoDomain + "/generatorBehavior"
	BuildAnnotationsGenAddHashSuffix  = konfig.ConfigAnnoDomain + "/needsHashSuffix"
)

var BuildAnnotations = []string{
	BuildAnnotationPreviousKinds,
	BuildAnnotationPreviousNames,
	BuildAnnotationPrefixes,
	BuildAnnotationSuffixes,
	BuildAnnotationPreviousNamespaces,
	BuildAnnotationsRefBy,
	BuildAnnotationsGenBehavior,
	BuildAnnotationsGenAddHashSuffix,
}

func makeBuiltinPlugin(r resid.Gvk) (resmap.Configurable, error) {
	bpt := transformers.GetBuiltinPluginType(r.Kind)
	if f, ok := transformers.TransformerFactories[bpt]; ok {
		return f(), nil
	}
	return nil, errors.Errorf("unable to load builtin %s", r)
}

func NewPluginHelpers() (*resmap.PluginHelpers, error) {
	depProvider := provider.NewDepProvider()

	fSys := filesys.MakeFsOnDisk()
	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	resmapFactory := resmap.NewFactory(depProvider.GetResourceFactory())

	lr := fLdr.RestrictionNone

	ldr, err := fLdr.NewLoader(lr, path, fSys)
	if err != nil {
		return nil, err
	}

	return resmap.NewPluginHelpers(ldr, depProvider.GetFieldValidator(), resmapFactory, types.DisabledPluginConfig()), nil
}

func RemoveBuildAnnotations(r *resource.Resource) {
	annotations := r.GetAnnotations()
	if len(annotations) == 0 {
		return
	}
	for _, a := range BuildAnnotations {
		delete(annotations, a)
	}
	if err := r.SetAnnotations(annotations); err != nil {
		panic(err)
	}
}

func main() {

	var processor framework.ResourceListProcessorFunc = func(rl *framework.ResourceList) error {

		config := rl.FunctionConfig

		res := resource.Resource{RNode: *config}

		plugin, err := makeBuiltinPlugin(resid.GvkFromNode(config))
		if err != nil {
			return errors.WrapPrefixf(err, "creating plugin")
		}

		yamlNode := config.YNode()
		yaml, err := yaml.Marshal(yamlNode)

		if err != nil {
			return errors.WrapPrefixf(err, "marshalling yaml from res %s", res.OrgId())
		}
		helpers, err := NewPluginHelpers()
		if err != nil {
			return errors.WrapPrefixf(err, "Cannot build Plugin helpers")
		}
		err = plugin.Config(helpers, yaml)
		if err != nil {
			return errors.WrapPrefixf(
				err, "plugin %s fails configuration", res.OrgId())
		}

		transformer, ok := plugin.(resmap.Transformer)
		if !ok {
			return fmt.Errorf("plugin %s not a transformer", res.OrgId())
		}

		rm, err := helpers.ResmapFactory().NewResMapFromRNodeSlice(rl.Items)
		if err != nil {
			return errors.WrapPrefixf(err, "getting resource maps")
		}
		err = transformer.Transform(rm)
		if err != nil {
			return errors.WrapPrefixf(err, "Transforming resources")
		}

		for _, r := range rm.Resources() {
			RemoveBuildAnnotations(r)
		}

		rl.Items = rm.ToRNodeSlice()

		return nil

	}

	cmd := command.Build(processor, command.StandaloneDisabled, false)
	command.AddGenerateDockerfile(cmd)
	cmd.Version = "v0.0.1" // <---VERSION--->

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
