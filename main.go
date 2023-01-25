package main

import (
	"fmt"
	"os"

	"github.com/kaweezle/krmfnbuiltin/pkg/plugins"
	"github.com/kaweezle/krmfnbuiltin/pkg/utils"

	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func main() {

	var processor framework.ResourceListProcessorFunc = func(rl *framework.ResourceList) error {

		config := rl.FunctionConfig

		res := resource.Resource{RNode: *config}

		plugin, err := plugins.MakeBuiltinPlugin(resid.GvkFromNode(config))
		if err != nil {
			return errors.WrapPrefixf(err, "creating plugin")
		}

		yamlNode := config.YNode()
		yamlBytes, err := yaml.Marshal(yamlNode)

		if err != nil {
			return errors.WrapPrefixf(err, "marshalling yaml from res %s", res.OrgId())
		}
		helpers, err := plugins.NewPluginHelpers()
		if err != nil {
			return errors.WrapPrefixf(err, "Cannot build Plugin helpers")
		}
		err = plugin.Config(helpers, yamlBytes)
		if err != nil {
			return errors.WrapPrefixf(
				err, "plugin %s fails configuration", res.OrgId())
		}

		transformer, ok := plugin.(resmap.Transformer)
		if ok {
			rm, err := helpers.ResmapFactory().NewResMapFromRNodeSlice(rl.Items)
			if err != nil {
				return errors.WrapPrefixf(err, "getting resource maps")
			}
			err = transformer.Transform(rm)
			if err != nil {
				return errors.WrapPrefixf(err, "Transforming resources")
			}

			for _, r := range rm.Resources() {
				utils.RemoveBuildAnnotations(r)
			}

			rl.Items = rm.ToRNodeSlice()

			// kustomize fn don't remove config.kubernetes.io/local-config resources upon completion.
			// As it always add a filename by default, the local resources keep saved.
			// To avoid this, an annotation `config.kubernetes.io/prune-local` present in a
			// transformer makes all the local resources disappear.
			if _, ok := config.GetAnnotations()["config.kubernetes.io/prune-local"]; ok {
				filter := &filters.IsLocalConfig{IncludeLocalConfig: false, ExcludeNonLocalConfig: false}
				err = rl.Filter(filter)
				if err != nil {
					return errors.WrapPrefixf(err, "filtering local configs")
				}
			}

		} else {
			generator, ok := plugin.(resmap.Generator)

			if !ok {
				return fmt.Errorf("plugin %s is neither a generator nor a transformer", res.OrgId())
			}

			rm, err := generator.Generate()
			if err != nil {
				return errors.WrapPrefixf(err, "generating resource(s)")
			}

			for _, r := range rm.Resources() {
				r.RemoveBuildAnnotations()
				// We add the annotation config.kubernetes.io/local-config to be able to delete
				// The generated resource at the end of the process. Unfortunately, kustomize doesn't
				// do that on functions. So we have added a special annotation
				// `config.kubernetes.io/prune-local` to add on the last transformer.
				// We set the filename of the generated resource in case it is forgotten.
				r.Pipe(yaml.SetAnnotation(filters.LocalConfigAnnotation, "true"))
				r.Pipe(yaml.SetAnnotation(kioutil.PathAnnotation, ".generated.yaml"))
				r.Pipe(yaml.SetAnnotation(kioutil.LegacyPathAnnotation, ".generated.yaml"))
			}

			rl.Items = append(rl.Items, rm.ToRNodeSlice()...)

		}

		return nil

	}

	cmd := command.Build(processor, command.StandaloneDisabled, false)
	command.AddGenerateDockerfile(cmd)
	cmd.Version = "v0.1.0" // <---VERSION--->

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
