package extras

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
	"github.com/pelletier/go-toml/v2"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Extender allows both traversing and modifying hierarchical opaque data
// structures like yaml, toml or ini files.
// Part of the structure is addressed through a path that is an array of string
// symbols.
//
//   - It is first initialized with SetPayload with the data structure payload.
//   - Traversal is done with Get
//   - Modification of part of the structure is done through Set
//   - After modification, the modified payload is retrieved with GetPayload
type Extender interface {
	// SetPayload initialize the embedded data structure with payload.
	SetPayload(payload []byte) error
	// GetPayload returns the current data structure in the appropriate encoding.
	GetPayload() ([]byte, error)
	// Get returns the subset of the structure at path in the appropriate encoding.
	Get(path []string) ([]byte, error)
	// Set modifies the data structure at path with value. Value can either be
	// in the appropriate encoding or can be encoded by the Extender. Please
	// see the Extender documentation to see how the the value is treated.
	Set(path []string, value any) error
}

// ExtendedSegment contains the path segment of a resource inside an embedded
// data structure.
type ExtendedSegment struct {
	Encoding string   // The encoding of the embedded data structure
	Path     []string // The path inside the embedded data structure
}

// String returns a string representation of the ExtendedSegment.
//
// For instance:
//
//	!!yaml.common.targetRevision
func (e *ExtendedSegment) String() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("!!%s", e.Encoding)
	} else {
		return fmt.Sprintf("!!%s.%s", e.Encoding, strings.Join(e.Path, "."))
	}
}

type any interface{}

// ExtenderType enumerates the existing extender types.
//
//go:generate go run golang.org/x/tools/cmd/stringer -type=ExtenderType
type ExtenderType int

const (
	Unknown ExtenderType = iota
	YamlExtender
	Base64Extender
	RegexExtender
	JsonExtender
	TomlExtender
	IniExtender
)

// stringToExtenderTypeMap maps encoding names to the corresponding extender
var stringToExtenderTypeMap map[string]ExtenderType

func init() { //nolint:gochecknoinits
	stringToExtenderTypeMap = makeStringToExtenderTypeMap()
}

// getByteValue returns value encoded as a byte array.
func getByteValue(value any) []byte {
	switch v := value.(type) {
	case *yaml.Node:
		return []byte(v.Value)
	case []byte:
		return v
	case string:
		return []byte(v)
	}
	return []byte{}
}

// makeStringToExtenderTypeMap makes a map to get the appropriate
// [ExtenderType] given its name.
func makeStringToExtenderTypeMap() (result map[string]ExtenderType) {
	result = make(map[string]ExtenderType, 3)
	for k := range ExtenderFactories {
		result[strings.Replace(strings.ToLower(k.String()), "extender", "", 1)] = k
	}
	return
}

// getExtenderType returns the appropriate [ExtenderType] for the passed
// extender type name
func getExtenderType(n string) ExtenderType {
	result, ok := stringToExtenderTypeMap[strings.ToLower(n)]
	if ok {
		return result
	}
	return Unknown
}

////////////////
// YAML Extender
////////////////

// yamlExtender manages embedded YAML in KRM resources.
//
// Internally, it uses a RNode. It avoids additional dependencies and preserves
// ordering and comments.
type yamlExtender struct {
	node *yaml.RNode
}

// parsePayload parses payload into a RNode.
//
// The payload can either by in YAML or JSON format.
func parsePayload(payload []byte) (*yaml.RNode, error) {
	nodes, err := (&kio.ByteReader{
		Reader:                bytes.NewBuffer(payload),
		OmitReaderAnnotations: false,
		PreserveSeqIndent:     true,
		WrapBareSeqNode:       true,
	}).Read()

	if err != nil {
		return nil, errors.WrapPrefixf(err, "while reading payload")
	}
	return nodes[0], nil
}

// SetPayload parses payload an sets the extender internal state
func (e *yamlExtender) SetPayload(payload []byte) (err error) {
	e.node, err = parsePayload(payload)
	return
}

// serializeNode serialize one node into YAML
func serializeNode(node *yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	err := (&kio.ByteWriter{Writer: &b}).Write([]*yaml.RNode{node})
	return b.Bytes(), err
}

// GetPayload returns the current payload in the proper encoding
func (e *yamlExtender) GetPayload() ([]byte, error) {
	return serializeNode(e.node)
}

// unwrapSeqNode unwraps node if it is a Wrapped Bare Seq Node
func unwrapSeqNode(node *yaml.RNode) *yaml.RNode {
	seqNode, err := node.Pipe(yaml.Lookup(yaml.BareSeqNodeWrappingKey))
	if err == nil && !seqNode.IsNilOrEmpty() {
		return seqNode
	}
	return node
}

// Lookup looks for the specified path in node and return the matching node. If
// kind is a valid node kind and the node doesn't exist, create it.
func Lookup(node *yaml.RNode, path []string, kind yaml.Kind) (*yaml.RNode, error) {
	// TODO: consider using yaml.PathGetter instead
	node, err := unwrapSeqNode(node).Pipe(&yaml.PathGetter{Path: path, Create: kind})
	if err != nil {
		return nil, errors.WrapPrefixf(err, "while getting path %s", strings.Join(path, "."))
	}

	return node, nil
}

// nodeSerializer is a RNode serializer function
type nodeSerializer func(*yaml.RNode) ([]byte, error)

// getNodePath returns the value of the node at path serialized with serializer.
func getNodePath(node *yaml.RNode, path []string, serializer nodeSerializer) ([]byte, error) {
	node, err := Lookup(node, path, 0)
	if err != nil {
		return nil, fmt.Errorf("error fetching elements in replacement target: %w", err)
	}

	if node.YNode().Kind == yaml.ScalarNode {
		return []byte(node.YNode().Value), nil
	}

	return serializer(node)
}

// Get returns the encoded payload at the specified path
func (e *yamlExtender) Get(path []string) ([]byte, error) {
	return getNodePath(e.node, path, serializeNode)
}

// setValue sets value at path on node
func setValue(node *yaml.RNode, path []string, value any) error {

	kind := yaml.ScalarNode
	if v, ok := value.(*yaml.Node); ok {
		kind = v.Kind
	}

	target, err := Lookup(node, path, kind)
	if err != nil {
		return fmt.Errorf("error fetching elements in replacement target: %w", err)
	}

	if target.YNode().Kind == yaml.ScalarNode {
		target.YNode().Value = string(getByteValue(value))
	} else {
		if target.YNode().Kind == kind {
			v, _ := value.(*yaml.Node)
			target.SetYNode(v)
		} else {
			return fmt.Errorf("setting non yaml object in place of object of type %s at path %s", target.YNode().Tag, strings.Join(path, "."))
		}
	}
	return nil
}

// Set modifies the current payload with value at the specified path.
func (e *yamlExtender) Set(path []string, value any) error {
	return setValue(e.node, path, value)
}

// NewYamlExtender returns a newly created YAML [Extender].
//
// With this encoding, you can set scalar values (strings, numbers) as well
// as mapping values.
func NewYamlExtender() Extender {
	return &yamlExtender{}
}

/////////
// Base64
/////////

// base64Extender manages embedded base64 in KRM resources.
type base64Extender struct {
	decoded []byte // The base64 decoded payload
}

// SetPayload decodes the payload and stores in internal state.
func (e *base64Extender) SetPayload(payload []byte) error {
	decoded, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return errors.WrapPrefixf(err, "while decoding base64")
	}
	e.decoded = decoded
	return nil
}

// GetPayload returns the current payload as base64
func (e *base64Extender) GetPayload() ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString(e.decoded)), nil
}

// Get returns the current base64 decoded payload.
//
// An error is returned if the path is not empty.
func (e *base64Extender) Get(path []string) ([]byte, error) {
	if len(path) > 0 {
		return nil, fmt.Errorf("path is invalid for base64: %s", strings.Join(path, "."))
	}
	return e.decoded, nil
}

// Set stores value in the current payload. path must be empty.
func (e *base64Extender) Set(path []string, value any) error {
	if len(path) > 0 {
		return fmt.Errorf("path is invalid for base64: %s", strings.Join(path, "."))
	}
	e.decoded = getByteValue(value)
	return nil
}

// NewBase64Extender returns a newly created Base64 extender.
//
// This extender doesn't allow structured traversal and modification. It just
// passes its decoded payload downstream. Example of usage:
//
//	prefix.!!base64.!!yaml.inside.path
//
// The above means that we want to modify inside.path in the YAML payload that
// is stored in base64 in the prefix property.
func NewBase64Extender() Extender {
	return &base64Extender{}
}

/////////
// Regex
////////

// regexExtender allows text replacement in pure text properties.
//
// see [NewRegexExtender]
type regexExtender struct {
	text []byte
}

// SetPayload store the plain payload internally
func (e *regexExtender) SetPayload(payload []byte) error {
	e.text = payload
	return nil
}

// GetPayload returns the text payload
func (e *regexExtender) GetPayload() ([]byte, error) {
	return []byte(e.text), nil
}

// Get returns the text matched by the regexp contained in the first segment of
// path.
func (e *regexExtender) Get(path []string) ([]byte, error) {
	if len(path) < 1 {
		return nil, fmt.Errorf("path for regex should at least be one")
	}
	re, err := regexp.Compile(path[0])
	if err != nil {
		return nil, fmt.Errorf("bad regex %s", path[0])
	}
	return re.Find(e.text), nil
}

// Set modifies the inner text inserting value in the capture group specified by
// path[1] of the Regexp specified by path[0].
//
// Example paths:
//
//	[`^\s+HostName\s+(\S+)\s*$`, `1`]
//
// Changes the value after HostName with value.
//
//	[`^\s+HostName\s+\S+\s*$`, `0`]
//
// Replace the whole line with value.
func (e *regexExtender) Set(path []string, value any) error {
	if len(path) != 2 {
		return fmt.Errorf("path for regex should at least be one")
	}
	re, err := regexp.Compile("(?m)" + path[0])
	if err != nil {
		return fmt.Errorf("bad regex %s", path[0])
	}

	group, err := strconv.Atoi(path[1])
	if err != nil {
		return fmt.Errorf("bad capturing group")
	}

	var b bytes.Buffer
	start := 0
	matched := false

	for _, v := range re.FindAllSubmatchIndex(e.text, -1) {
		matched = true
		startIndex := group * 2

		b.Write(e.text[start:v[startIndex]])
		b.Write(getByteValue(value))
		start = v[startIndex+1]
	}

	if matched {
		if start < len(e.text) {
			b.Write(e.text[start:len(e.text)])
		}
		e.text = b.Bytes()
	}

	return nil
}

// NewRegexExtender returns a newly created Regexp [Extender].
//
// This extender allows text replacement in pure text properties. It is useful
// in the case the content of the KRM property is not structured.
//
// We don't recommend using it too much as it weakens the transformation.
//
// The paths to use with this extender are always composed of two elements:
//
//   - The regexp to look for in the text.
//   - The capture group index to replace with the source value.
//
// Examples:
//
//	^\s+HostName\s+(\S+)\s*$.1
//
// Changes the value after HostName with value.
//
//	^\s+HostName\s+\S+\s*$.0
//
// Replace the whole line with value.
func NewRegexExtender() Extender {
	return &regexExtender{}
}

///////
// JSON
///////

// jsonExtender is an [Extender] allowing modifications in JSON content.
//
// It is close to [yamlExtender] as kyaml knows to read and write JSON files.
type jsonExtender struct {
	node *yaml.RNode
}

// SetPayload parses the JSON payload and stores it internally as a yaml.RNode.
func (e *jsonExtender) SetPayload(payload []byte) (err error) {
	e.node, err = parsePayload(payload)
	return
}

// getJSONPayload returns the JSON payload for the passed node.
//
// There is a small issue in kio.ByteWriter preventing the JSON serialization of
// a wrapped JSON array.
func getJSONPayload(node *yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	if node.YNode().Kind == yaml.MappingNode {
		node = node.Copy()
		node.Pipe(yaml.ClearAnnotation(kioutil.IndexAnnotation))
		node.Pipe(yaml.ClearAnnotation(kioutil.LegacyIndexAnnotation))
		node.Pipe(yaml.ClearAnnotation(kioutil.SeqIndentAnnotation))
		yaml.ClearEmptyAnnotations(node)
	}
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "  ")
	err := errors.Wrap(encoder.Encode(node))

	return b.Bytes(), err
}

// GetPayload returns the payload as a serialized JSON object
func (e *jsonExtender) GetPayload() ([]byte, error) {
	return getJSONPayload(unwrapSeqNode(e.node))
}

// Get returns the sub JSON specified by path.
func (e *jsonExtender) Get(path []string) ([]byte, error) {
	return getNodePath(e.node, path, getJSONPayload)
}

// Set modifies the inner JSON at path with value
func (e *jsonExtender) Set(path []string, value any) error {
	return setValue(e.node, path, value)
}

// NewJsonExtender returns a newly created [Extender] to modify JSON content.
//
// As with the YAML extender (see [NewYamlExtender]), modifications are not
// limited to scalar values but the source can be a mapping or a sequence.
func NewJsonExtender() Extender {
	return &jsonExtender{}
}

///////
// TOML
///////

// tomlExtender is an [Extender] allowing the structured modification of a TOML
// property.
type tomlExtender struct {
	node *yaml.RNode
}

// SetPayload sets the internal state with the TOML source payload.
func (e *tomlExtender) SetPayload(payload []byte) error {

	m := map[string]interface{}{}
	err := toml.Unmarshal(payload, &m)
	if err != nil {
		return errors.WrapPrefixf(err, "while un-marshalling toml")
	}

	e.node, err = yaml.FromMap(m)
	if err != nil {
		return errors.WrapPrefixf(err, "while converting into yaml")
	}

	return nil
}

// getTOMLPayload returns the TOML representation of the specified node.
//
// The node must be a mapping node.
func getTOMLPayload(node *yaml.RNode) ([]byte, error) {
	m, err := node.Map()
	if err != nil {
		return nil, errors.WrapPrefixf(err, "while encoding to map")
	}
	return toml.Marshal(m)
}

// GetPayload return the current payload as a TOML snippet.
func (e *tomlExtender) GetPayload() ([]byte, error) {
	return getTOMLPayload(e.node)
}

// Get returns the TOML representation of the sub element at path.
func (e *tomlExtender) Get(path []string) ([]byte, error) {
	return getNodePath(e.node, path, getTOMLPayload)
}

// Set modifies the current payload at path with value.
func (e *tomlExtender) Set(path []string, value any) error {
	return setValue(e.node, path, value)
}

// NewTomlExtender returns a newly created [Extender] for modifying properties
// containing TOML.
//
// Please be aware that this [Extender] doesn't preserve the source ordering
// nor the comments in the content.
func NewTomlExtender() Extender {
	return &tomlExtender{}
}

//////
// INI
//////

// iniExtender allows structured modification of ini file based properties.
type iniExtender struct {
	file *ini.File
}

// SetPayload parses payload as a INI file and set the internal state.
func (e *iniExtender) SetPayload(payload []byte) (err error) {

	e.file, err = ini.Load(payload)
	return err
}

// GetPayload returns the current state as an ini file.
func (e *iniExtender) GetPayload() ([]byte, error) {
	var b bytes.Buffer
	_, err := e.file.WriteTo(&b)
	return b.Bytes(), err
}

// keyFromPath returns the INI key at path.
func (e *iniExtender) keyFromPath(path []string) (*ini.Key, error) {
	if len(path) < 1 || len(path) > 2 {
		return nil, fmt.Errorf("invalid path length: %d", len(path))
	}
	section := ""
	key := path[0]
	if len(path) == 2 {
		section = key
		key = path[1]
	}
	return e.file.Section(section).Key(key), nil
}

// Get returns the content of the key specified by path.
func (e *iniExtender) Get(path []string) ([]byte, error) {
	k, err := e.keyFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("while getting key at path %s", strings.Join(path, "."))
	}
	return []byte(k.String()), nil
}

// Set sets the value of the key specified by path with value.
func (e *iniExtender) Set(path []string, value any) error {
	k, err := e.keyFromPath(path)
	if err != nil {
		return fmt.Errorf("while getting key at path %s", strings.Join(path, "."))
	}

	k.SetValue(string(getByteValue(value)))

	return nil
}

// NewIniExtender returns a newly created [Extender] for modifying INI files
// like properties.
//
// Some tools may use ini type configuration files. This extender allows
// modification of the values. At this point, it doesn't allow inserting
// complete sections. If paths have one element, it will set the corresponding
// property at the root level. If path have two elements, the first one contains
// the section name and the second the property name.
//
// Please be aware that this [Extender] doesn't preserve the source ordering
// nor the comments in the content.
func NewIniExtender() Extender {
	return &iniExtender{}
}

////////////
// Factories
////////////

// ExtenderFactories register the [Extender] factory functions for each
// [ExtenderType].
var ExtenderFactories = map[ExtenderType]func() Extender{
	YamlExtender:   NewYamlExtender,
	Base64Extender: NewBase64Extender,
	RegexExtender:  NewRegexExtender,
	JsonExtender:   NewJsonExtender,
	TomlExtender:   NewTomlExtender,
	IniExtender:    NewIniExtender,
}

// Extender returns a newly created [Extender] for the appropriate encoding.
// uses [ExtenderFactories].
func (path *ExtendedSegment) Extender(payload []byte) (Extender, error) {
	bpt := getExtenderType(path.Encoding)
	if f, ok := ExtenderFactories[bpt]; ok {
		result := f()
		if err := result.SetPayload(payload); err != nil {
			return nil, err
		}

		return result, nil
	}
	return nil, errors.Errorf("unable to load extender %s", path.Encoding)
}

///////////////
// ExtendedPath
///////////////

// splitExtendedPath fills extensions with the ExtendedSegments found in path
// and returns the path prefix. This method is used by [NewExtendedPath]
func splitExtendedPath(path []string, extensions *[]*ExtendedSegment) (basePath []string, err error) {

	if len(path) == 0 {
		return
	}

	for i, p := range path {
		if strings.HasPrefix(p, "!!") {
			extension := ExtendedSegment{Encoding: p[2:]}
			if extension.Encoding == "" {
				err = fmt.Errorf("extension cannot be empty")
				return
			}
			*extensions = append(*extensions, &extension)
			var remainder []string
			remainder, err = splitExtendedPath(path[i+1:], extensions)
			if err != nil {
				err = errors.WrapPrefixf(err, "while getting subpath of extension %s", extension.Encoding)
				return
			}
			extension.Path = remainder
			return
		} else {
			basePath = append(basePath, p)
		}
	}
	return
}

// ExtendedPath contains all the paths segments of a path.
// The path is composed by:
//
//   - a KRM resource path, the prefix (ResourcePath)
//   - 0 or more [ExtendedSegment]s.
//
// For instance, for the following path:
//
//	data.secretConfiguration.!!base64.!!yaml.common.URL
//
// ResourcePath would be ["data", "secretConfiguration"] and ExtendedSegments:
//
//	*[]*ExtendedSegment{
//	     &ExtendedSegment{Encoding: "base64", Path: []string{}},
//	     &ExtendedSegment{Encoding: "yaml", Path: []string{"common", "URL"}},
//	 }
type ExtendedPath struct {
	// ResourcePath is The KRM portion of the path
	ResourcePath []string
	// ExtendedSegments contains all extended path segments
	ExtendedSegments *[]*ExtendedSegment
}

// NewExtendedPath creates an [ExtendedPath] from the split path segments in paths.
func NewExtendedPath(path []string) (*ExtendedPath, error) {
	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	if err != nil {
		return nil, errors.WrapPrefixf(err, "while getting extended path")
	}

	return &ExtendedPath{ResourcePath: prefix, ExtendedSegments: &extensions}, nil
}

// HasExtensions returns true if the path contains extended segments.
func (ep *ExtendedPath) HasExtensions() bool {
	return len(*ep.ExtendedSegments) > 0
}

// String returns a string representation of the extended path.
func (ep *ExtendedPath) String() string {
	out := strings.Join(ep.ResourcePath, ".")
	if len(*ep.ExtendedSegments) > 0 {
		segmentStrings := []string{}
		for _, s := range *ep.ExtendedSegments {
			segmentStrings = append(segmentStrings, s.String())
		}
		out = fmt.Sprintf("%s.%s", out, strings.Join(segmentStrings, "."))
	}
	return out
}

// applyIndex applies value to input starting at the extended path index.
func (ep *ExtendedPath) applyIndex(index int, input []byte, value *yaml.Node) ([]byte, error) {
	if index >= len(*ep.ExtendedSegments) || index < 0 {
		return nil, fmt.Errorf("invalid extended path index: %d", index)
	}

	segment := (*ep.ExtendedSegments)[index]
	extender, err := segment.Extender(input)
	if err != nil {
		return nil, errors.WrapPrefixf(err, "creating extender at index: %d", index)
	}

	if index == len(*ep.ExtendedSegments)-1 {
		err := extender.Set(segment.Path, value)
		if err != nil {
			return nil, errors.WrapPrefixf(err, "setting value on path %s", segment.String())
		}
	} else {
		nextInput, err := extender.Get(segment.Path)
		if err != nil {
			return nil, errors.WrapPrefixf(err, "getting value on path %s", segment.String())
		}
		newValue, err := ep.applyIndex(index+1, nextInput, value)
		if err != nil {
			return nil, err
		}

		err = extender.Set(segment.Path, newValue)
		if err != nil {
			return nil, errors.WrapPrefixf(err, "setting value on path %s", segment.String())
		}
	}
	return extender.GetPayload()
}

// Apply applies value to target. target is the KRM resource specified by
// ResourcePrefix.
//
// Apply creates the appropriate [Extender] for each extended segment and
// traverse it until the last. When reaching the last, it sets value
// in the appropriate path. It then unwinds the paths and save the modified
// value in the target.
func (ep *ExtendedPath) Apply(target *yaml.RNode, value *yaml.RNode) error {
	if target.YNode().Kind != yaml.ScalarNode {
		return fmt.Errorf("extended path only works on scalar nodes")
	}

	outValue := value.YNode().Value
	if len(*ep.ExtendedSegments) > 0 {
		input := []byte(target.YNode().Value)
		output, err := ep.applyIndex(0, input, value.YNode())
		if err != nil {
			return errors.WrapPrefixf(err, "applying value on extended segment %s", ep.String())
		}

		outValue = string(output)
	}
	target.YNode().Value = outValue
	return nil
}
