package plugins

import (
	"os"

	"github.com/kaweezle/krmfnbuiltin/pkg/extras"
	"sigs.k8s.io/kustomize/api/builtins"
	"sigs.k8s.io/kustomize/api/filesys"
	fLdr "sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/resid"
)

//go:generate go run golang.org/x/tools/cmd/stringer -type=BuiltinPluginType
type BuiltinPluginType int

const (
	Unknown BuiltinPluginType = iota
	AnnotationsTransformer
	ConfigMapGenerator
	IAMPolicyGenerator
	HashTransformer
	ImageTagTransformer
	LabelTransformer
	NamespaceTransformer
	PatchJson6902Transformer
	PatchStrategicMergeTransformer
	PatchTransformer
	PrefixSuffixTransformer
	PrefixTransformer
	SuffixTransformer
	ReplicaCountTransformer
	SecretGenerator
	ValueAddTransformer
	HelmChartInflationGenerator
	ReplacementTransformer
	GitConfigMapGenerator
	RemoveTransformer
	KustomizationGenerator
)

var stringToBuiltinPluginTypeMap map[string]BuiltinPluginType

func init() { //nolint:gochecknoinits
	stringToBuiltinPluginTypeMap = makeStringToBuiltinPluginTypeMap()
}

func makeStringToBuiltinPluginTypeMap() (result map[string]BuiltinPluginType) {
	result = make(map[string]BuiltinPluginType, 23)
	for k := range TransformerFactories {
		result[k.String()] = k
	}
	for k := range GeneratorFactories {
		result[k.String()] = k
	}
	return
}

func GetBuiltinPluginType(n string) BuiltinPluginType {
	result, ok := stringToBuiltinPluginTypeMap[n]
	if ok {
		return result
	}
	return Unknown
}

type MultiTransformer struct {
	transformers []resmap.TransformerPlugin
}

func (t *MultiTransformer) Transform(m resmap.ResMap) error {
	for _, transformer := range t.transformers {
		if err := transformer.Transform(m); err != nil {
			return err
		}
	}
	return nil
}

func (t *MultiTransformer) Config(h *resmap.PluginHelpers, b []byte) error {
	for _, transformer := range t.transformers {
		if err := transformer.Config(h, b); err != nil {
			return err
		}
	}
	return nil
}

func NewMultiTransformer() resmap.TransformerPlugin {
	return &MultiTransformer{[]resmap.TransformerPlugin{
		builtins.NewPrefixTransformerPlugin(),
		builtins.NewSuffixTransformerPlugin(),
	}}
}

var TransformerFactories = map[BuiltinPluginType]func() resmap.TransformerPlugin{
	AnnotationsTransformer:         builtins.NewAnnotationsTransformerPlugin,
	HashTransformer:                builtins.NewHashTransformerPlugin,
	ImageTagTransformer:            builtins.NewImageTagTransformerPlugin,
	LabelTransformer:               builtins.NewLabelTransformerPlugin,
	NamespaceTransformer:           builtins.NewNamespaceTransformerPlugin,
	PatchJson6902Transformer:       builtins.NewPatchJson6902TransformerPlugin,
	PatchStrategicMergeTransformer: builtins.NewPatchStrategicMergeTransformerPlugin,
	PatchTransformer:               builtins.NewPatchTransformerPlugin,
	PrefixSuffixTransformer:        NewMultiTransformer,
	PrefixTransformer:              builtins.NewPrefixTransformerPlugin,
	SuffixTransformer:              builtins.NewSuffixTransformerPlugin,
	ReplacementTransformer:         extras.NewExtendedReplacementTransformerPlugin,
	ReplicaCountTransformer:        builtins.NewReplicaCountTransformerPlugin,
	ValueAddTransformer:            builtins.NewValueAddTransformerPlugin,
	RemoveTransformer:              extras.NewRemoveTransformerPlugin,
	// Do not wired SortOrderTransformer as a builtin plugin.
	// We only want it to be available in the top-level kustomization.
	// See: https://github.com/kubernetes-sigs/kustomize/issues/3913
}

var GeneratorFactories = map[BuiltinPluginType]func() resmap.GeneratorPlugin{
	ConfigMapGenerator:          builtins.NewConfigMapGeneratorPlugin,
	IAMPolicyGenerator:          builtins.NewIAMPolicyGeneratorPlugin,
	SecretGenerator:             builtins.NewSecretGeneratorPlugin,
	HelmChartInflationGenerator: builtins.NewHelmChartInflationGeneratorPlugin,
	GitConfigMapGenerator:       extras.NewGitConfigMapGeneratorPlugin,
	KustomizationGenerator:      extras.NewKustomizationGeneratorPlugin,
}

func MakeBuiltinPlugin(r resid.Gvk) (resmap.Configurable, error) {
	bpt := GetBuiltinPluginType(r.Kind)
	if f, ok := TransformerFactories[bpt]; ok {
		return f(), nil
	}
	if f, ok := GeneratorFactories[bpt]; ok {
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
	resmapFactory.RF().IncludeLocalConfigs = true

	lr := fLdr.RestrictionNone

	ldr, err := fLdr.NewLoader(lr, path, fSys)
	if err != nil {
		return nil, err
	}

	return resmap.NewPluginHelpers(ldr, depProvider.GetFieldValidator(), resmapFactory, types.DisabledPluginConfig()), nil
}
