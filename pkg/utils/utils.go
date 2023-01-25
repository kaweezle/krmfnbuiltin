package utils

import "sigs.k8s.io/kustomize/api/resource"

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
