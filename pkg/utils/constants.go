package utils

import "sigs.k8s.io/kustomize/api/konfig"

const (
	// build annotations
	BuildAnnotationPreviousKinds      = konfig.ConfigAnnoDomain + "/previousKinds"
	BuildAnnotationPreviousNames      = konfig.ConfigAnnoDomain + "/previousNames"
	BuildAnnotationPrefixes           = konfig.ConfigAnnoDomain + "/prefixes"
	BuildAnnotationSuffixes           = konfig.ConfigAnnoDomain + "/suffixes"
	BuildAnnotationPreviousNamespaces = konfig.ConfigAnnoDomain + "/previousNamespaces"
	BuildAnnotationsRefBy             = konfig.ConfigAnnoDomain + "/refBy"
	BuildAnnotationsGenBehavior       = konfig.ConfigAnnoDomain + "/generatorBehavior"
	BuildAnnotationsGenAddHashSuffix  = konfig.ConfigAnnoDomain + "/needsHashSuffix"

	// ConfigurationAnnotationDomain is the domain of function configuration
	// annotations
	ConfigurationAnnotationDomain = "config.kubernetes.io"

	LocalConfigurationAnnotationDomain = "config.kaweezle.com"

	// Function configuration annotation
	FunctionAnnotationFunction = ConfigurationAnnotationDomain + "/function"

	// true when the resource is part of the local configuration
	FunctionAnnotationLocalConfig = LocalConfigurationAnnotationDomain + "/local-config"

	// Setting to true means we want this function configuration to be injected as a
	// local configuration resource (local-config)
	FunctionAnnotationInjectLocal = LocalConfigurationAnnotationDomain + "/inject-local"

	// if set, the transformation will remove all the resources marked as local-config
	FunctionAnnotationPruneLocal = LocalConfigurationAnnotationDomain + "/prune-local"
	// if set on a Generated resource, it won't be pruned
	FunctionAnnotationKeepLocal = LocalConfigurationAnnotationDomain + "/keep-local"
	FunctionAnnotationPath      = LocalConfigurationAnnotationDomain + "/path"
	FunctionAnnotationIndex     = LocalConfigurationAnnotationDomain + "/index"
)
