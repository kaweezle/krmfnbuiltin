package extras

import (
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/yaml"
)

// KustomizationGeneratorPlugin configures the KustomizationGenerator.
type KustomizationGeneratorPlugin struct {
	Directory string `json:"kustomizeDirectory,omitempty" yaml:"kustomizeDirectory,omitempty"`
}

// enablePlugins adds to opts the options to run exec functions
func enablePlugins(opts *krusty.Options) *krusty.Options {
	opts.PluginConfig = types.EnabledPluginConfig(types.BploUseStaticallyLinked) // cSpell: disable-line
	opts.PluginConfig.FnpLoadingOptions.EnableExec = true
	opts.PluginConfig.FnpLoadingOptions.AsCurrentUser = true
	opts.PluginConfig.HelmConfig.Command = "helm"
	opts.LoadRestrictions = types.LoadRestrictionsNone
	return opts
}

// runKustomizations runs the kustomization in dirname (URL compatible) with
// the filesystem fs.
func runKustomizations(fs filesys.FileSystem, dirname string) (resources resmap.ResMap, err error) {

	opts := enablePlugins(krusty.MakeDefaultOptions())
	k := krusty.MakeKustomizer(opts)
	resources, err = k.Run(fs, dirname)
	return
}

// Config reads the function configuration, i.e. the kustomizeDirectory
func (p *KustomizationGeneratorPlugin) Config(
	h *resmap.PluginHelpers, c []byte) (err error) {
	err = yaml.Unmarshal(c, p)
	if err != nil {
		return err
	}
	return err
}

// Generate generates the resources of the directory
func (p *KustomizationGeneratorPlugin) Generate() (resmap.ResMap, error) {
	return runKustomizations(filesys.MakeFsOnDisk(), p.Directory)
}

// NewKustomizationGeneratorPlugin returns a newly Created KustomizationGenerator
func NewKustomizationGeneratorPlugin() resmap.GeneratorPlugin {
	return &KustomizationGeneratorPlugin{}
}
