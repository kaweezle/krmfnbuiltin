package extras

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/yaml"
)

type RemoveTransformerPlugin struct {
	Targets []*types.Selector `json:"targets,omitempty" yaml:"targets,omitempty"`
}

func (p *RemoveTransformerPlugin) Config(
	h *resmap.PluginHelpers, c []byte) (err error) {
	err = yaml.Unmarshal(c, p)
	if err != nil {
		return err
	}
	return err
}

func (p *RemoveTransformerPlugin) Transform(m resmap.ResMap) error {
	if p.Targets == nil {
		return fmt.Errorf("must specify at least one target")
	}
	for _, t := range p.Targets {
		resources, err := m.Select(*t)
		if err != nil {
			return errors.WrapPrefixf(err, "while selecting target %s", t.String())
		}
		for _, r := range resources {
			err = m.Remove(r.CurId())
			if err != nil {
				return errors.WrapPrefixf(err, "while removing resource %s", r.CurId().String())
			}
		}
	}
	return nil
}

func NewRemoveTransformerPlugin() resmap.TransformerPlugin {
	return &RemoveTransformerPlugin{}
}
