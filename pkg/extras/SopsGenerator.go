package extras

import (
	"fmt"

	"github.com/kaweezle/krmfnbuiltin/pkg/utils"
	"github.com/pkg/errors"
	"go.mozilla.org/sops/v3/aes"
	"go.mozilla.org/sops/v3/cmd/sops/common"
	"go.mozilla.org/sops/v3/cmd/sops/formats"
	"go.mozilla.org/sops/v3/keyservice"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/kyaml/kio"
	kiof "sigs.k8s.io/kustomize/kyaml/kio/filters"
	yaml "sigs.k8s.io/kustomize/kyaml/yaml"
	oyaml "sigs.k8s.io/yaml"
)

const (
	defaultApiVersion = "config.kaweezle.com/v1alpha1"
	defaultKind       = "PlatformSecrets"
	c
)

// SopsGeneratorPlugin configures the SopsGenerator.
type SopsGeneratorPlugin struct {
	yaml.ResourceMeta

	Spec struct {
		// Replicas is the number of Deployment replicas
		// Defaults to the REPLICAS env var, or 1
		Files []string `yaml:"files,omitempty"`
	} `yaml:"spec,omitempty"`

	Sops map[string]interface{} `json:"sops,omitempty" yaml:"spec,omitempty"`

	h      *resmap.PluginHelpers
	buffer []byte
}

func Decrypt(b []byte, format formats.Format, file string, ignoreMac bool) (nodes []*yaml.RNode, err error) {

	store := common.StoreForFormat(format)

	// Load SOPS file and access the data key
	tree, err := store.LoadEncryptedFile(b)
	if err != nil {
		return nil, err
	}

	_, err = common.DecryptTree(common.DecryptTreeOpts{
		KeyServices: []keyservice.KeyServiceClient{
			keyservice.NewLocalClient(),
		},
		Tree:      &tree,
		IgnoreMac: ignoreMac,
		Cipher:    aes.NewCipher(),
	})

	if err != nil {
		return nil, err
	}

	var data []byte

	data, err = store.EmitPlainFile(tree.Branches)
	if err != nil {
		err = errors.Wrapf(err, "trouble decrypting file %s", file)
		return
	}

	nodes, err = kio.FromBytes(data)
	if err != nil {
		err = errors.Wrapf(err, "Error while reading decrypted resources from file %s", file)
	}
	return
}

// Config reads the function configuration, i.e. the kustomizeDirectory
func (p *SopsGeneratorPlugin) Config(h *resmap.PluginHelpers, c []byte) (err error) {
	err = oyaml.Unmarshal(c, p)
	if err != nil {
		return
	}
	p.h = h
	if p.Sops != nil {
		p.buffer = c
	} else {
		if p.Spec.Files == nil {
			err = fmt.Errorf("generator configuration doesn't contain any file")
			return
		}
	}
	return
}

// Generate generates the resources of the directory
func (p *SopsGeneratorPlugin) Generate() (resmap.ResMap, error) {
	var nodes []*yaml.RNode
	if p.buffer != nil {
		name := p.GetIdentifier().Name
		var err error
		nodes, err = Decrypt(p.buffer, formats.Yaml, name, true)
		if err != nil {
			return nil, errors.Wrapf(err, "error decoding manifest %q, content -->%s<--", name, string(p.buffer))
		}
		kind := defaultKind
		apiVersion := defaultApiVersion
		if p.Annotations != nil {
			if annoKind, ok := p.Annotations[utils.FunctionAnnotationKind]; ok {
				kind = annoKind
			}
			if annoApiVersion, ok := p.Annotations[utils.FunctionAnnotationApiVersion]; ok {
				apiVersion = annoApiVersion
			}
		}

		for _, r := range nodes {
			r.SetKind(kind)
			r.SetApiVersion(apiVersion)
			if err := r.PipeE(yaml.ClearAnnotation(utils.FunctionAnnotationFunction)); err != nil {
				return nil, err
			}
			if err := r.PipeE(yaml.ClearAnnotation(kiof.LocalConfigAnnotation)); err != nil {
				return nil, err
			}
		}
	} else {

		for _, file := range p.Spec.Files {

			b, err := p.h.Loader().Load(file)
			if err != nil {
				return nil, errors.Wrapf(err, "error reading manifest %q", file)
			}

			format := formats.FormatForPath(file)
			fileNodes, err := Decrypt(b, format, file, false)
			if err != nil {
				return nil, errors.Wrapf(err, "error decrypting file %q", file)
			}
			nodes = append(nodes, fileNodes...)

		}

	}
	return p.h.ResmapFactory().NewResMapFromRNodeSlice(nodes)
}

// NewSopsGeneratorPlugin returns a newly Created SopsGenerator
func NewSopsGeneratorPlugin() resmap.GeneratorPlugin {
	return &SopsGeneratorPlugin{}
}
