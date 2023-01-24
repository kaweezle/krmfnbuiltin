package extras

import (
	"bytes"
	"testing"

	"github.com/lithammer/dedent"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/kustomize/kyaml/kio"
	kyaml_utils "sigs.k8s.io/kustomize/kyaml/utils"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type ExtenderTestSuite struct {
	suite.Suite
}

func (s *ExtenderTestSuite) SetupTest() {
}

func (s *ExtenderTestSuite) TeardownTest() {
}

func (s *ExtenderTestSuite) TestSplitPath() {
	require := s.Require()
	p := "toto.tata.!!yaml.toto.tata"
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	remainder, err := splitExtendedPath(path, &extensions)

	require.NoError(err)
	require.Len(remainder, 2, "Remainder path should be 2")
	require.Len(extensions, 1, "Should only have one extension")
	require.Equal("yaml", extensions[0].Encoding, "Extension should be yaml")
	require.Len(extensions[0].Path, 2, "Extension path len should be 2")
}

func (s *ExtenderTestSuite) TestRegexExtender() {
	text := dedent.Dedent(`
    PubkeyAcceptedKeyTypes +ssh-rsa
    Host sishserver
      HostName holepunch.in
      Port 2222
      BatchMode yes
      IdentityFile ~/.ssh_keys/id_rsa
      IdentitiesOnly yes
      LogLevel ERROR
      ServerAliveInterval 10
      ServerAliveCountMax 2
      RemoteCommand sni-proxy=true
      RemoteForward citest.holepunch.in:443 traefik.traefik.svc:443
    `)
	expected := dedent.Dedent(`
    PubkeyAcceptedKeyTypes +ssh-rsa
    Host sishserver
      HostName kaweezle.com
      Port 2222
      BatchMode yes
      IdentityFile ~/.ssh_keys/id_rsa
      IdentitiesOnly yes
      LogLevel ERROR
      ServerAliveInterval 10
      ServerAliveCountMax 2
      RemoteCommand sni-proxy=true
      RemoteForward citest.holepunch.in:443 traefik.traefik.svc:443
    `)
	require := s.Require()
	path := &ExtendedSegment{
		Encoding: "regex",
		Path:     []string{`^\s+HostName\s+(\S+)\s*$`, `1`},
	}

	extender, err := path.Extender([]byte(text))
	require.NoError(err)
	require.NotNil(extender)

	require.NoError(extender.Set(path.Path, []byte("kaweezle.com")))

	out, err := extender.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(out), "Text should be modified")
}

func (s *ExtenderTestSuite) TestBase64Extender() {
	encoded := "UHVia2V5QWNjZXB0ZWRLZXlUeXBlcyArc3NoLXJzYQpIb3N0IHNpc2hzZXJ2ZXIKICBIb3N0TmFtZSBob2xlcHVuY2guaW4KICBQb3J0IDIyMjIKICBCYXRjaE1vZGUgeWVzCiAgSWRlbnRpdHlGaWxlIH4vLnNzaF9rZXlzL2lkX3JzYQogIElkZW50aXRpZXNPbmx5IHllcwogIExvZ0xldmVsIEVSUk9SCiAgU2VydmVyQWxpdmVJbnRlcnZhbCAxMAogIFNlcnZlckFsaXZlQ291bnRNYXggMgogIFJlbW90ZUNvbW1hbmQgc25pLXByb3h5PXRydWUKICBSZW1vdGVGb3J3YXJkIGNpdGVzdC5ob2xlcHVuY2guaW46NDQzIHRyYWVmaWsudHJhZWZpay5zdmM6NDQzCg=="
	decodedExpected := dedent.Dedent(`
	PubkeyAcceptedKeyTypes +ssh-rsa
	Host sishserver
	  HostName holepunch.in
	  Port 2222
	  BatchMode yes
	  IdentityFile ~/.ssh_keys/id_rsa
	  IdentitiesOnly yes
	  LogLevel ERROR
	  ServerAliveInterval 10
	  ServerAliveCountMax 2
	  RemoteCommand sni-proxy=true
	  RemoteForward citest.holepunch.in:443 traefik.traefik.svc:443
	`)[1:]

	modifiedEncoded := "UHVia2V5QWNjZXB0ZWRLZXlUeXBlcyArc3NoLXJzYQpIb3N0IHNpc2hzZXJ2ZXIKICBIb3N0TmFtZSBrYXdlZXpsZS5jb20KICBQb3J0IDIyMjIKICBCYXRjaE1vZGUgeWVzCiAgSWRlbnRpdHlGaWxlIH4vLnNzaF9rZXlzL2lkX3JzYQogIElkZW50aXRpZXNPbmx5IHllcwogIExvZ0xldmVsIEVSUk9SCiAgU2VydmVyQWxpdmVJbnRlcnZhbCAxMAogIFNlcnZlckFsaXZlQ291bnRNYXggMgogIFJlbW90ZUNvbW1hbmQgc25pLXByb3h5PXRydWUKICBSZW1vdGVGb3J3YXJkIGNpdGVzdC5ob2xlcHVuY2guaW46NDQzIHRyYWVmaWsudHJhZWZpay5zdmM6NDQzCg=="

	require := s.Require()

	p := `!!base64.!!regex.\s+HostName\s+(\S+).1`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 2, "There should be 2 extensions")
	require.Equal("base64", extensions[0].Encoding, "The first extension should be base64")

	b64Ext := extensions[0]
	b64Extender, err := b64Ext.Extender([]byte(encoded))
	require.NoError(err)
	require.IsType(&base64Extender{}, b64Extender, "Should be a base64 extender")

	decoded, err := b64Extender.Get(b64Ext.Path)
	require.NoError(err)
	require.Equal(decodedExpected, string(decoded), "bad base64 decoding")

	regexExt := extensions[1]
	reExtender, err := regexExt.Extender(decoded)
	require.NoError(err)
	require.IsType(&regexExtender{}, reExtender, "Should be a regex extender")

	require.NoError(reExtender.Set(regexExt.Path, []byte("kaweezle.com")))
	modified, err := reExtender.GetPayload()
	require.NoError(err)
	require.NoError(b64Extender.Set(b64Ext.Path, modified))
	final, err := b64Extender.GetPayload()
	require.NoError(err)
	require.Equal(modifiedEncoded, string(final), "final base64 is bad")
}

func (s *ExtenderTestSuite) TestYamlExtender() {
	require := s.Require()
	source := dedent.Dedent(`
    uninode: true
    common:
      targetRevision: main
    apps:
      enabled: true
    `)[1:]
	expected := dedent.Dedent(`
    uninode: true
    common:
      targetRevision: deploy/citest
    apps:
      enabled: true
    `)[1:]

	p := `!!yaml.common.targetRevision`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("yaml", extensions[0].Encoding, "The first extension should be base64")

	yamlXP := extensions[0]
	yamlExt, err := yamlXP.Extender([]byte(source))
	require.NoError(err)
	value, err := yamlExt.Get(yamlXP.Path)
	require.NoError(err)
	require.Equal("main", string(value), "error fetching value")
	require.NoError(yamlExt.Set(yamlXP.Path, []byte("deploy/citest")))

	modified, err := yamlExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final yaml")

	value, err = yamlExt.Get(yamlXP.Path)
	require.NoError(err)
	require.Equal("deploy/citest", string(value), "error fetching changed value")
}

func (s *ExtenderTestSuite) TestYamlExtenderWithSequence() {
	require := s.Require()
	source := dedent.Dedent(`
    - name: common.targetRevision
      value: main
    - name: common.repoURL
      value: https://github.com/antoinemartin/autocloud.git
    `)[1:]
	expected := dedent.Dedent(`
    - name: common.targetRevision
      value: deploy/citest
    - name: common.repoURL
      value: https://github.com/antoinemartin/autocloud.git
    `)[1:]

	p := `!!yaml.[name=common.targetRevision].value`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("yaml", extensions[0].Encoding, "The first extension should be base64")

	yamlXP := extensions[0]
	yamlExt, err := yamlXP.Extender([]byte(source))
	require.NoError(err)
	require.NoError(yamlExt.Set(yamlXP.Path, []byte("deploy/citest")))

	modified, err := yamlExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final yaml")
}

func (s *ExtenderTestSuite) TestYamlExtenderWithYaml() {
	require := s.Require()
	sources, err := (&kio.ByteReader{Reader: bytes.NewBufferString(`
common: |
  uninode: true
  common:
    targetRevision: main
  apps:
    enabled: true
`)}).Read()
	require.NoError(err)
	require.Len(sources, 1)
	source := sources[0]

	expected := dedent.Dedent(`
    common: |
      uninode: true
      common:
        targetRevision: deploy/citest
        repoURL: https://github.com/antoinemartin/autocloud.git
      apps:
        enabled: true
    `)[1:]

	replacements, err := (&kio.ByteReader{Reader: bytes.NewBufferString(`
common:
    targetRevision: deploy/citest
    repoURL: https://github.com/antoinemartin/autocloud.git
`)}).Read()
	require.NoError(err)
	require.Len(replacements, 1)
	replacement := replacements[0]

	p := `common.!!yaml.common`
	path := kyaml_utils.SmarterPathSplitter(p, ".")
	e, err := NewExtendedPath(path)
	require.NoError(err)
	require.Len(e.resourcePath, 1, "no resource path")

	sourcePath := []string{"common"}

	target, err := source.Pipe(&yaml.PathGetter{Path: e.resourcePath})
	require.NoError(err)

	value, err := replacement.Pipe(&yaml.PathGetter{Path: sourcePath})
	require.NoError(err)
	err = e.Apply(target, value)
	require.NoError(err)

	var b bytes.Buffer
	err = (&kio.ByteWriter{Writer: &b}).Write(sources)
	require.NoError(err)

	sString, err := source.String()
	require.NoError(err)
	require.Equal(expected, b.String(), sString, "replacement failed")
}

func (s *ExtenderTestSuite) TestJsonExtender() {
	require := s.Require()
	source := `{
  "common": {
    "targetRevision": "main"
  },
  "uninode": true,
  "apps": {
    "enabled": true
  }
}`
	expected := `{
  "apps": {
    "enabled": true
  },
  "common": {
    "targetRevision": "deploy/citest"
  },
  "uninode": true
}
`

	p := `!!json.common.targetRevision`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("json", extensions[0].Encoding, "The first extension should be json")

	jsonXP := extensions[0]
	jsonExt, err := jsonXP.Extender([]byte(source))
	require.NoError(err)
	value, err := jsonExt.Get(jsonXP.Path)
	require.NoError(err)
	require.Equal("main", string(value), "error fetching value")
	require.NoError(jsonExt.Set(jsonXP.Path, []byte("deploy/citest")))

	modified, err := jsonExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final json")

	value, err = jsonExt.Get(jsonXP.Path)
	require.NoError(err)
	require.Equal("deploy/citest", string(value), "error fetching changed value")
}

func (s *ExtenderTestSuite) TestJsonArrayExtender() {
	require := s.Require()
	source := `[
  {
    "name": "targetRevision",
    "value": "main"
  },
  {
    "name": "repoURL",
    "value": "https://github.com/kaweezle/example.git"
  }
]`
	expected := `[
  {
    "name": "targetRevision",
    "value": "deploy/citest"
  },
  {
    "name": "repoURL",
    "value": "https://github.com/kaweezle/example.git"
  }
]
`

	p := `!!json.[name=targetRevision].value`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("json", extensions[0].Encoding, "The first extension should be json")

	jsonXP := extensions[0]
	jsonExt, err := jsonXP.Extender([]byte(source))
	require.NoError(err)
	value, err := jsonExt.Get(jsonXP.Path)
	require.NoError(err)
	require.Equal("main", string(value), "error fetching value")
	require.NoError(jsonExt.Set(jsonXP.Path, []byte("deploy/citest")))

	modified, err := jsonExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final json")

	value, err = jsonExt.Get(jsonXP.Path)
	require.NoError(err)
	require.Equal("deploy/citest", string(value), "error fetching changed value")
}

func (s *ExtenderTestSuite) TestTomlExtender() {
	require := s.Require()
	source := `
uninode = true
[common]
targetRevision = 'main'
[apps]
enabled = true
`
	expected := `uninode = true

[apps]
enabled = true

[common]
targetRevision = 'deploy/citest'
`

	p := `!!toml.common.targetRevision`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("toml", extensions[0].Encoding, "The first extension should be toml")

	tomlXP := extensions[0]
	tomlExt, err := tomlXP.Extender([]byte(source))
	require.NoError(err)
	value, err := tomlExt.Get(tomlXP.Path)
	require.NoError(err)
	require.Equal("main", string(value), "error fetching value")
	require.NoError(tomlExt.Set(tomlXP.Path, []byte("deploy/citest")))

	modified, err := tomlExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final toml")

	value, err = tomlExt.Get(tomlXP.Path)
	require.NoError(err)
	require.Equal("deploy/citest", string(value), "error fetching changed value")
}

func (s *ExtenderTestSuite) TestIniExtender() {
	require := s.Require()
	source := `
uninode = true
[common]
targetRevision = main
[apps]
enabled = true
`
	expected := `uninode = true

[common]
targetRevision = deploy/citest

[apps]
enabled = true
`

	p := `!!ini.common.targetRevision`
	path := kyaml_utils.SmarterPathSplitter(p, ".")

	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	require.NoError(err)
	require.Len(prefix, 0, "There should be no prefix")
	require.Len(extensions, 1, "There should be 2 extensions")
	require.Equal("ini", extensions[0].Encoding, "The first extension should be ini")

	iniXP := extensions[0]
	iniExt, err := iniXP.Extender([]byte(source))
	require.NoError(err)
	value, err := iniExt.Get(iniXP.Path)
	require.NoError(err)
	require.Equal("main", string(value), "error fetching value")
	require.NoError(iniExt.Set(iniXP.Path, []byte("deploy/citest")))

	modified, err := iniExt.GetPayload()
	require.NoError(err)
	require.Equal(expected, string(modified), "final ini")

	value, err = iniExt.Get(iniXP.Path)
	require.NoError(err)
	require.Equal("deploy/citest", string(value), "error fetching changed value")
}

func TestExtender(t *testing.T) {
	suite.Run(t, new(ExtenderTestSuite))
}
