// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package extras

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kaweezle/krmfnbuiltin/pkg/utils"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/api/loader"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/resid"
	kyaml_utils "sigs.k8s.io/kustomize/kyaml/utils"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type extendedFilter struct {
	Replacements []types.Replacement `json:"replacements,omitempty" yaml:"replacements,omitempty"`
	sourceNodes  []*yaml.RNode
}

// Filter replaces values of targets with values from sources
func (f extendedFilter) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	sourceNodes := f.sourceNodes
	if sourceNodes == nil {
		sourceNodes = nodes
	}
	for i, r := range f.Replacements {
		if r.Source == nil || r.Targets == nil {
			return nil, fmt.Errorf("replacements must specify a source and at least one target")
		}
		value, err := getReplacement(sourceNodes, &f.Replacements[i])
		if err != nil {
			return nil, err
		}
		nodes, err = applyReplacement(nodes, value, r.Targets)
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func getReplacement(nodes []*yaml.RNode, r *types.Replacement) (*yaml.RNode, error) {
	source, err := selectSourceNode(nodes, r.Source)
	if err != nil {
		return nil, err
	}

	if r.Source.FieldPath == "" {
		r.Source.FieldPath = types.DefaultReplacementFieldPath
	}
	fieldPath := kyaml_utils.SmarterPathSplitter(r.Source.FieldPath, ".")

	rn, err := source.Pipe(yaml.Lookup(fieldPath...))
	if err != nil {
		return nil, fmt.Errorf("error looking up replacement source: %w", err)
	}
	if rn.IsNilOrEmpty() {
		return nil, fmt.Errorf("fieldPath `%s` is missing for replacement source %s", r.Source.FieldPath, r.Source.ResId)
	}

	return getRefinedValue(r.Source.Options, rn)
}

// selectSourceNode finds the node that matches the selector, returning
// an error if multiple or none are found
func selectSourceNode(nodes []*yaml.RNode, selector *types.SourceSelector) (*yaml.RNode, error) {
	var matches []*yaml.RNode
	for _, n := range nodes {
		ids, err := makeResIds(n)
		if err != nil {
			return nil, fmt.Errorf("error getting node IDs: %w", err)
		}
		for _, id := range ids {
			if id.IsSelectedBy(selector.ResId) {
				if len(matches) > 0 {
					return nil, fmt.Errorf(
						"multiple matches for selector %s", selector)
				}
				matches = append(matches, n)
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("nothing selected by %s", selector)
	}
	return matches[0], nil
}

func getRefinedValue(options *types.FieldOptions, rn *yaml.RNode) (*yaml.RNode, error) {
	if options == nil || (options.Delimiter == "" && options.Encoding == "") {
		return rn, nil
	}
	if rn.YNode().Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("delimiter or encoding option can only be used with scalar nodes")
	}
	n := rn.Copy()
	if options.Delimiter != "" {
		value := strings.Split(yaml.GetValue(rn), options.Delimiter)
		if options.Index >= len(value) || options.Index < 0 {
			return nil, fmt.Errorf("options.index %d is out of bounds for value %s", options.Index, yaml.GetValue(rn))
		}

		n.YNode().Value = value[options.Index]
	} else {
		value, err := GetEncodedValue(yaml.GetValue(rn), options.Encoding)
		if err != nil {
			return nil, errors.Wrapf(err, "while encoding value")
		}
		n.YNode().Value = value
	}
	return n, nil
}

func applyReplacement(nodes []*yaml.RNode, value *yaml.RNode, targetSelectors []*types.TargetSelector) ([]*yaml.RNode, error) {
	for _, selector := range targetSelectors {
		if selector.Select == nil {
			return nil, errors.New("target must specify resources to select")
		}
		if len(selector.FieldPaths) == 0 {
			selector.FieldPaths = []string{types.DefaultReplacementFieldPath}
		}
		for _, possibleTarget := range nodes {
			ids, err := makeResIds(possibleTarget)
			if err != nil {
				return nil, err
			}

			// filter targets by label and annotation selectors
			selectByAnnoAndLabel, err := selectByAnnoAndLabel(possibleTarget, selector)
			if err != nil {
				return nil, err
			}
			if !selectByAnnoAndLabel {
				continue
			}

			// filter targets by matching resource IDs
			for i, id := range ids {
				if id.IsSelectedBy(selector.Select.ResId) && !rejectId(selector.Reject, &ids[i]) {
					err := copyValueToTarget(possibleTarget, value, selector)
					if err != nil {
						return nil, err
					}
					break
				}
			}
		}
	}
	return nodes, nil
}

func selectByAnnoAndLabel(n *yaml.RNode, t *types.TargetSelector) (bool, error) {
	if matchesSelect, err := matchesAnnoAndLabelSelector(n, t.Select); !matchesSelect || err != nil {
		return false, err
	}
	for _, reject := range t.Reject {
		if reject.AnnotationSelector == "" && reject.LabelSelector == "" {
			continue
		}
		if m, err := matchesAnnoAndLabelSelector(n, reject); m || err != nil {
			return false, err
		}
	}
	return true, nil
}

func matchesAnnoAndLabelSelector(n *yaml.RNode, selector *types.Selector) (bool, error) {
	r := resource.Resource{RNode: *n}
	annoMatch, err := r.MatchesAnnotationSelector(selector.AnnotationSelector)
	if err != nil {
		return false, err
	}
	labelMatch, err := r.MatchesLabelSelector(selector.LabelSelector)
	if err != nil {
		return false, err
	}
	return annoMatch && labelMatch, nil
}

func rejectId(rejects []*types.Selector, id *resid.ResId) bool {
	for _, r := range rejects {
		if !r.ResId.IsEmpty() && id.IsSelectedBy(r.ResId) {
			return true
		}
	}
	return false
}

func copyValueToTarget(target *yaml.RNode, value *yaml.RNode, selector *types.TargetSelector) error {
	for _, fp := range selector.FieldPaths {
		fieldPath := kyaml_utils.SmarterPathSplitter(fp, ".")
		extendedPath, err := NewExtendedPath(fieldPath)
		if err != nil {
			return err
		}
		create, err := shouldCreateField(selector.Options, extendedPath.ResourcePath)
		if err != nil {
			return err
		}

		var targetFields []*yaml.RNode
		if create {
			createdField, createErr := target.Pipe(yaml.LookupCreate(value.YNode().Kind, extendedPath.ResourcePath...))
			if createErr != nil {
				return fmt.Errorf("error creating replacement node: %w", createErr)
			}
			targetFields = append(targetFields, createdField)
		} else {
			// may return multiple fields, always wrapped in a sequence node
			foundFieldSequence, lookupErr := target.Pipe(&yaml.PathMatcher{Path: extendedPath.ResourcePath})
			if lookupErr != nil {
				return fmt.Errorf("error finding field in replacement target: %w", lookupErr)
			}
			targetFields, err = foundFieldSequence.Elements()
			if err != nil {
				return fmt.Errorf("error fetching elements in replacement target: %w", err)
			}
		}

		for _, t := range targetFields {
			if err := setFieldValue(selector.Options, t, value, extendedPath); err != nil {
				return err
			}
		}

	}
	return nil
}

func setFieldValue(options *types.FieldOptions, targetField *yaml.RNode, value *yaml.RNode, extendedPath *ExtendedPath) error {
	value = value.Copy()
	if options != nil && options.Delimiter != "" {
		if extendedPath.HasExtensions() {
			return fmt.Errorf("delimiter option cannot be used with extensions")
		}
		if targetField.YNode().Kind != yaml.ScalarNode {
			return fmt.Errorf("delimiter option can only be used with scalar nodes")
		}
		tv := strings.Split(targetField.YNode().Value, options.Delimiter)
		v := yaml.GetValue(value)
		// TODO: Add a way to remove an element
		switch {
		case options.Index < 0: // prefix
			tv = append([]string{v}, tv...)
		case options.Index >= len(tv): // suffix
			tv = append(tv, v)
		default: // replace an element
			tv[options.Index] = v
		}
		value.YNode().Value = strings.Join(tv, options.Delimiter)
	}

	if targetField.YNode().Kind == yaml.ScalarNode {
		return extendedPath.Apply(targetField, value)
	} else {
		if extendedPath.HasExtensions() {
			return fmt.Errorf("path extensions should start at a scalar node")
		}

		targetField.SetYNode(value.YNode())
	}

	return nil
}

func shouldCreateField(options *types.FieldOptions, fieldPath []string) (bool, error) {
	if options == nil || !options.Create {
		return false, nil
	}
	// create option is not supported in a wildcard matching
	for _, f := range fieldPath {
		if f == "*" {
			return false, fmt.Errorf("cannot support create option in a multi-value target")
		}
	}
	return true, nil
}

// Copied

// makeResIds returns all of an RNode's current and previous Ids
func makeResIds(n *yaml.RNode) ([]resid.ResId, error) {
	var result []resid.ResId
	apiVersion := n.Field(yaml.APIVersionField)
	var group, version string
	if apiVersion != nil {
		group, version = resid.ParseGroupVersion(yaml.GetValue(apiVersion.Value))
	}
	result = append(result, resid.NewResIdWithNamespace(
		resid.Gvk{Group: group, Version: version, Kind: n.GetKind()}, n.GetName(), n.GetNamespace()),
	)
	prevIds, err := prevIds(n)
	if err != nil {
		return nil, err
	}
	result = append(result, prevIds...)
	return result, nil
}

// prevIds returns all of an RNode's previous Ids
func prevIds(n *yaml.RNode) ([]resid.ResId, error) {
	var ids []resid.ResId
	// TODO: merge previous names and namespaces into one list of
	//     pairs on one annotation so there is no chance of error
	annotations := n.GetAnnotations()
	if _, ok := annotations[utils.BuildAnnotationPreviousNames]; !ok {
		return nil, nil
	}
	names := strings.Split(annotations[utils.BuildAnnotationPreviousNames], ",")
	ns := strings.Split(annotations[utils.BuildAnnotationPreviousNamespaces], ",")
	kinds := strings.Split(annotations[utils.BuildAnnotationPreviousKinds], ",")
	// This should never happen
	if len(names) != len(ns) || len(names) != len(kinds) {
		return nil, fmt.Errorf(
			"number of previous names, " +
				"number of previous namespaces, " +
				"number of previous kinds not equal")
	}
	for i := range names {
		meta, err := n.GetMeta()
		if err != nil {
			return nil, err
		}
		group, version := resid.ParseGroupVersion(meta.APIVersion)
		gvk := resid.Gvk{
			Group:   group,
			Version: version,
			Kind:    kinds[i],
		}
		ids = append(ids, resid.NewResIdWithNamespace(
			gvk, names[i], ns[i]))
	}
	return ids, nil
}

// plugin

// Replace values in targets with values from a source. This transformer is
// "extended" because it allows structured replacement in properties
// containing a string representation of some structured content. It currently
// supports the following structured formats:
//
//   - Yaml
//   - Json
//   - Toml
//   - Ini
//
// It also provides helpers for changing content in base64 encoded properties
// as well as a simple regexp based replacer for edge cases.
//
// Configuration of replacements can be found in the [kustomize doc].
//
// [kustomize doc]: https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/replacements/
type ExtendedReplacementTransformerPlugin struct {
	ReplacementList []types.ReplacementField `json:"replacements,omitempty" yaml:"replacements,omitempty"`
	Replacements    []types.Replacement      `json:"omitempty" yaml:"omitempty"`
	Source          string                   `json:"source,omitempty" yaml:"source,omitempty"`
	h               *resmap.PluginHelpers
}

// Config configures the plugin
func (p *ExtendedReplacementTransformerPlugin) Config(
	h *resmap.PluginHelpers, c []byte) (err error) {
	p.ReplacementList = []types.ReplacementField{}
	if err := yaml.Unmarshal(c, p); err != nil {
		return err
	}
	p.h = h

	for _, r := range p.ReplacementList {
		if r.Path != "" && (r.Source != nil || len(r.Targets) != 0) {
			return fmt.Errorf("cannot specify both path and inline replacement")
		}
		if r.Path != "" {
			// load the replacement from the path
			content, err := h.Loader().Load(r.Path)
			if err != nil {
				return err
			}
			// find if the path contains a a list of replacements or a single replacement
			var replacement interface{}
			err = yaml.Unmarshal(content, &replacement)
			if err != nil {
				return err
			}
			items := reflect.ValueOf(replacement)
			switch items.Kind() {
			case reflect.Slice:
				repl := []types.Replacement{}
				if err := yaml.Unmarshal(content, &repl); err != nil {
					return err
				}
				p.Replacements = append(p.Replacements, repl...)
			case reflect.Map:
				repl := types.Replacement{}
				if err := yaml.Unmarshal(content, &repl); err != nil {
					return err
				}
				p.Replacements = append(p.Replacements, repl)
			default:
				return fmt.Errorf("unsupported replacement type encountered within replacement path: %v", items.Kind())
			}
		} else {
			// replacement information is already loaded
			p.Replacements = append(p.Replacements, r.Replacement)
		}
	}

	return nil
}

// Transform performs the configured replacements in the specified resource map
func (p *ExtendedReplacementTransformerPlugin) Transform(m resmap.ResMap) (err error) {

	var sourceNodes []*yaml.RNode
	if p.Source != "" {
		source, err := p.h.ResmapFactory().FromFile(p.h.Loader(), p.Source)
		if err != nil {
			if errors.Is(err, loader.ErrHTTP) {
				return errors.Wrapf(err, "while reading source %s", p.Source)
			}

			source, err = runKustomizations(filesys.MakeFsOnDisk(), p.Source)
			if err != nil {
				return errors.Wrapf(err, "while getting source for replacements %s", p.Source)
			}
		}

		sourceNodes = source.ToRNodeSlice()
	}

	return m.ApplyFilter(extendedFilter{
		Replacements: p.Replacements,
		sourceNodes:  sourceNodes,
	})
}

// NewExtendedReplacementTransformerPlugin returns a newly created [ExtendedReplacementTransformerPlugin]
func NewExtendedReplacementTransformerPlugin() resmap.TransformerPlugin {
	return &ExtendedReplacementTransformerPlugin{}
}
