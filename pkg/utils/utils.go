package utils

import (
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

func MakeResourceLocal(r *yaml.RNode) error {
	annotations := r.GetAnnotations()

	annotations[FunctionAnnotationLocalConfig] = "true"
	if _, ok := annotations[kioutil.PathAnnotation]; !ok {
		annotations[kioutil.PathAnnotation] = ".generated.yaml"
	}
	if _, ok := annotations[kioutil.LegacyPathAnnotation]; !ok {
		annotations[kioutil.LegacyPathAnnotation] = ".generated.yaml"
	}
	delete(annotations, FunctionAnnotationInjectLocal)
	delete(annotations, FunctionAnnotationFunction)

	return r.SetAnnotations(annotations)
}

func unLocal(list []*yaml.RNode) ([]*yaml.RNode, error) {
	output := []*yaml.RNode{}
	for _, r := range list {
		annotations := r.GetAnnotations()
		if _, ok := annotations[FunctionAnnotationKeepLocal]; ok {
			delete(annotations, FunctionAnnotationKeepLocal)
			delete(annotations, FunctionAnnotationLocalConfig)
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
		} else {
			if _, ok := annotations[FunctionAnnotationLocalConfig]; !ok {
				output = append(output, r)
			}
		}
	}
	return output, nil
}

var UnLocal kio.FilterFunc = unLocal
