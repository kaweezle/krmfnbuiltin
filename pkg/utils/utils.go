package utils

import (
	"strconv"

	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var buildAnnotations = []string{
	BuildAnnotationPreviousKinds,
	BuildAnnotationPreviousNames,
	BuildAnnotationPrefixes,
	BuildAnnotationSuffixes,
	BuildAnnotationPreviousNamespaces,
	BuildAnnotationsRefBy,
	BuildAnnotationsGenBehavior,
	BuildAnnotationsGenAddHashSuffix,
}

// RemoveBuildAnnotations removes kustomize build annotations from r.
//
// Contrary to the method available in resource.Resource, this method doesn't
// remove the file name related annotations, as this would prevent modification
// of the source file.
func RemoveBuildAnnotations(r *resource.Resource) {
	annotations := r.GetAnnotations()
	if len(annotations) == 0 {
		return
	}
	for _, a := range buildAnnotations {
		delete(annotations, a)
	}
	if err := r.SetAnnotations(annotations); err != nil {
		panic(err)
	}
}

func TransferAnnotations(list []*yaml.RNode, config *yaml.RNode) (err error) {
	path := ".krmfnbuiltin.yaml"
	startIndex := 0

	configAnnotations := config.GetAnnotations()
	_, local := configAnnotations[FunctionAnnotationLocalConfig]

	if annoPath, ok := configAnnotations[FunctionAnnotationPath]; ok {
		path = annoPath
	}

	if annoIndex, ok := configAnnotations[FunctionAnnotationIndex]; ok {
		startIndex, err = strconv.Atoi(annoIndex)
		if err != nil {
			return
		}
	}

	for index, r := range list {
		annotations := r.GetAnnotations()
		if local {
			annotations[FunctionAnnotationLocalConfig] = "true"
		}
		if path != "" {
			annotations[kioutil.LegacyPathAnnotation] = path
			annotations[kioutil.PathAnnotation] = path

			curIndex := strconv.Itoa(startIndex + index)
			annotations[kioutil.LegacyIndexAnnotation] = curIndex
			annotations[kioutil.IndexAnnotation] = curIndex
		}
		delete(annotations, FunctionAnnotationInjectLocal)
		delete(annotations, FunctionAnnotationFunction)
		r.SetAnnotations(annotations)
	}
	return
}

func unLocal(list []*yaml.RNode) ([]*yaml.RNode, error) {
	output := []*yaml.RNode{}
	for _, r := range list {
		annotations := r.GetAnnotations()
		// We don't append resources with config.kaweezle.com/local-config resources
		if _, ok := annotations[FunctionAnnotationLocalConfig]; !ok {
			// For the remaining resources, if a path and/or index was specified
			// we copy it.
			if path, ok := annotations[FunctionAnnotationPath]; ok {
				annotations[kioutil.LegacyPathAnnotation] = path
				annotations[kioutil.PathAnnotation] = path
				delete(annotations, FunctionAnnotationPath)
			}
			if index, ok := annotations[FunctionAnnotationIndex]; ok {
				annotations[kioutil.LegacyIndexAnnotation] = index
				annotations[kioutil.IndexAnnotation] = index
				delete(annotations, FunctionAnnotationIndex)
			}
			r.SetAnnotations(annotations)
			output = append(output, r)
		}
	}
	return output, nil
}

var UnLocal kio.FilterFunc = unLocal
