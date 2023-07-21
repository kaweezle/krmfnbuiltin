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
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func main() {

	var processor framework.ResourceListProcessorFunc = func(rl *framework.ResourceList) error {

		config := rl.FunctionConfig

		res := resource.Resource{RNode: *config}

		plugin, err := plugins.MakeBuiltinPlugin(resid.GvkFromNode(config))
		if err != nil {
			// Check if config asks us to inject it
			if _, ok := config.GetAnnotations()[utils.FunctionAnnotationInjectLocal]; !ok {
				return errors.WrapPrefixf(err, "creating plugin")
			}
		}

		ok := false
		var transformer resmap.Transformer

		if plugin != nil {
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

			transformer, ok = plugin.(resmap.Transformer)
		}

		if ok {
			rm := utils.ResourceMapFromNodes(rl.Items)
			err = transformer.Transform(rm)
			if err != nil {
				return errors.WrapPrefixf(err, "Transforming resources")
			}

			configAnnotations := config.GetAnnotations()

			if _, ok := configAnnotations[utils.FunctionAnnotationCleanup]; ok {
				for _, r := range rm.Resources() {
					utils.RemoveBuildAnnotations(r)
				}
			}

			rl.Items = rm.ToRNodeSlice()

			// If the annotation `config.kaweezle.com/prune-local` is present in a
			// transformer makes all the local resources disappear.
			if _, ok := configAnnotations[utils.FunctionAnnotationPruneLocal]; ok {
				err = rl.Filter(utils.UnLocal)
				if err != nil {
					return errors.WrapPrefixf(err, "while pruning `config.kaweezle.com/local-config` resources")
				}
			}

		} else {
			var rrl []*yaml.RNode
			if plugin == nil { // No plugin, it's an heredoc document
				rrl = []*yaml.RNode{config.Copy()}
			} else {
				generator, ok := plugin.(resmap.Generator)

				if !ok {
					return fmt.Errorf("plugin %s is neither a generator nor a transformer", res.OrgId())
				}

				rm, err := generator.Generate()
				if err != nil {
					return errors.WrapPrefixf(err, "generating resource(s)")
				}

				rrl = rm.ToRNodeSlice()
			}

			if err := utils.TransferAnnotations(rrl, config); err != nil {
				return errors.WrapPrefixf(err, "While transferring annotations")
			}

			rl.Items = append(rl.Items, rrl...)

		}

		return nil

	}

	cmd := command.Build(processor, command.StandaloneDisabled, false)
	command.AddGenerateDockerfile(cmd)
	cmd.Version = "v0.4.3" // <---VERSION--->

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
