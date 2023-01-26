package extras

import (
	"fmt"

	git "github.com/go-git/go-git/v5"
	"sigs.k8s.io/kustomize/api/kv"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/yaml"
)

// GitConfigMapGeneratorPlugin generates a config map that includes two
// properties of the current git repository:
//
//   - repoURL contains the URL or the remote specified by remoteName. by
//     default, it takes the URL of the remote named "origin".
//   - targetRevision contains the name of the current branch.
//
// This generator is useful in transformations that use those values, like for
// instance Argo CD application customization.
//
// Information about the configuration can be found in the [kustomize doc].
//
// [kustomize doc]: https://kubectl.docs.kubernetes.io/references/kustomize/builtins/#_configmapgenerator_
type GitConfigMapGeneratorPlugin struct {
	h                *resmap.PluginHelpers
	types.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	types.ConfigMapArgs
	// The name of the remote which URL to include. defaults to "origin".
	RemoteName string `json:"remoteName,omitempty" yaml:"remoteName,omitempty"`
}

// Config configures the generator with the functionConfig passed in config.
func (p *GitConfigMapGeneratorPlugin) Config(h *resmap.PluginHelpers, config []byte) (err error) {
	p.ConfigMapArgs = types.ConfigMapArgs{}
	err = yaml.Unmarshal(config, p)
	if p.ConfigMapArgs.Name == "" {
		p.ConfigMapArgs.Name = p.Name
	}
	if p.ConfigMapArgs.Namespace == "" {
		p.ConfigMapArgs.Namespace = p.Namespace
	}
	p.h = h
	return
}

// Generate generates the config map
func (p *GitConfigMapGeneratorPlugin) Generate() (resmap.ResMap, error) {
	// Add git repository properties

	repo, err := git.PlainOpenWithOptions(p.h.Loader().Root(), &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return resmap.New(), errors.WrapPrefixf(err, "opening git repo")
	}
	// TODO: Should come from config
	remoteName := p.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}
	origin, err := repo.Remote(remoteName)
	if err != nil {
		return resmap.New(), errors.WrapPrefixf(err, "getting remote %s", remoteName)
	}

	p.ConfigMapArgs.KvPairSources.LiteralSources = append(p.ConfigMapArgs.KvPairSources.LiteralSources,
		fmt.Sprintf("repoURL=%s", origin.Config().URLs[0]))

	head, err := repo.Head()
	if err != nil {
		return resmap.New(), errors.WrapPrefixf(err, "getting current branch")
	}

	p.ConfigMapArgs.KvPairSources.LiteralSources = append(p.ConfigMapArgs.KvPairSources.LiteralSources,
		fmt.Sprintf("targetRevision=%s", head.Name().Short()))

	return p.h.ResmapFactory().FromConfigMapArgs(
		kv.NewLoader(p.h.Loader(), p.h.Validator()), p.ConfigMapArgs)
}

// NewGitConfigMapGeneratorPlugin returns a newly created GitConfigMapGenerator.
func NewGitConfigMapGeneratorPlugin() resmap.GeneratorPlugin {
	return &GitConfigMapGeneratorPlugin{}
}
