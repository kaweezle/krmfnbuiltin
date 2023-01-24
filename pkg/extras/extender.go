package extras

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type Extender interface {
	SetPayload(payload []byte) error
	GetPayload() ([]byte, error)
	Get(path []string) ([]byte, error)
	Set(path []string, value any) error
}

type ExtendedSegment struct {
	Encoding string
	Path     []string
}

func (e *ExtendedSegment) String() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("!!%s", e.Encoding)
	} else {
		return fmt.Sprintf("!!%s.%s", e.Encoding, strings.Join(e.Path, "."))
	}
}

type any interface{}

//go:generate go run golang.org/x/tools/cmd/stringer -type=ExtenderType
type ExtenderType int

const (
	Unknown ExtenderType = iota
	YamlExtender
	Base64Extender
	RegexExtender
	JsonExtender
)

var stringToExtenderTypeMap map[string]ExtenderType

func init() { //nolint:gochecknoinits
	stringToExtenderTypeMap = makeStringToExtenderTypeMap()
}

func GetByteValue(value any) []byte {
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

func makeStringToExtenderTypeMap() (result map[string]ExtenderType) {
	result = make(map[string]ExtenderType, 3)
	for k := range ExtenderFactories {
		result[k.String()] = k
	}
	return
}

func GetExtenderType(n string) ExtenderType {
	result, ok := stringToExtenderTypeMap[n]
	if ok {
		return result
	}
	return Unknown
}

////////////////
// YAML Extender
////////////////

type yamlExtender struct {
	node *yaml.RNode
}

func readPayload(payload []byte) (*yaml.RNode, error) {
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

func (e *yamlExtender) SetPayload(payload []byte) (err error) {
	e.node, err = readPayload(payload)
	return
}

func getNodeBytes(nodes []*yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	err := (&kio.ByteWriter{Writer: &b}).Write(nodes)
	return b.Bytes(), err
}

func (e *yamlExtender) GetPayload() ([]byte, error) {
	return getNodeBytes([]*yaml.RNode{e.node})
}

func LookupNode(node *yaml.RNode) *yaml.RNode {
	seqNode, err := node.Pipe(yaml.Lookup(yaml.BareSeqNodeWrappingKey))
	if err == nil && !seqNode.IsNilOrEmpty() {
		return seqNode
	}
	return node
}

func Lookup(node *yaml.RNode, path []string) ([]*yaml.RNode, error) {
	node, err := LookupNode(node).Pipe(&yaml.PathMatcher{Path: path})
	if err != nil {
		return nil, errors.WrapPrefixf(err, "while getting path %s", strings.Join(path, "."))
	}

	return node.Elements()
}

func (e *yamlExtender) Get(path []string) ([]byte, error) {
	targetFields, err := Lookup(e.node, path)
	if err != nil {
		return nil, fmt.Errorf("error fetching elements in replacement target: %w", err)
	}

	if len(targetFields) == 1 && targetFields[0].YNode().Kind == yaml.ScalarNode {
		return []byte(targetFields[0].YNode().Value), nil
	}

	return getNodeBytes(targetFields)
}

func (e *yamlExtender) Set(path []string, value any) error {

	targetFields, err := Lookup(e.node, path)
	if err != nil {
		return fmt.Errorf("error fetching elements in replacement target: %w", err)
	}

	for _, t := range targetFields {
		if t.YNode().Kind == yaml.ScalarNode {
			t.YNode().Value = string(GetByteValue(value))
		} else {
			if v, ok := value.(*yaml.Node); ok {
				t.SetYNode(v)
			} else {
				return fmt.Errorf("setting non yaml object in place of object of type %s at path %s", t.YNode().Tag, strings.Join(path, "."))
			}
		}
	}
	return nil
}

func NewYamlExtender() Extender {
	return &yamlExtender{}
}

/////////
// Base64
/////////

type base64Extender struct {
	decoded []byte
}

func (e *base64Extender) SetPayload(payload []byte) error {
	decoded, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return errors.WrapPrefixf(err, "while decoding base64")
	}
	e.decoded = decoded
	return nil
}

func (e *base64Extender) GetPayload() ([]byte, error) {
	return e.decoded, nil
}

func (e *base64Extender) Get(path []string) ([]byte, error) {
	if len(path) > 0 {
		return nil, fmt.Errorf("path is invalid for base64: %s", strings.Join(path, "."))
	}
	return e.decoded, nil
}

func (e *base64Extender) Set(path []string, value any) error {
	if len(path) > 0 {
		return fmt.Errorf("path is invalid for base64: %s", strings.Join(path, "."))
	}
	e.decoded = []byte(base64.StdEncoding.EncodeToString(GetByteValue(value)))
	return nil
}

func NewBase64Extender() Extender {
	return &base64Extender{}
}

/////////
// Regex
////////

type regexExtender struct {
	text []byte
}

func (e *regexExtender) SetPayload(payload []byte) error {
	e.text = payload
	return nil
}

func (e *regexExtender) GetPayload() ([]byte, error) {
	return []byte(e.text), nil
}

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
		b.Write(GetByteValue(value))
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

func NewRegexExtender() Extender {
	return &regexExtender{}
}

///////
// JSON
///////

type jsonExtender struct {
	node *yaml.RNode
}

func (e *jsonExtender) SetPayload(payload []byte) (err error) {
	e.node, err = readPayload(payload)
	return
}

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

func (e *jsonExtender) GetPayload() ([]byte, error) {
	return getJSONPayload(LookupNode(e.node))
}

func (e *jsonExtender) Get(path []string) ([]byte, error) {
	targetFields, err := Lookup(e.node, path)
	if err != nil {
		return nil, errors.WrapPrefixf(err, "error fetching elements in replacement target")
	}

	if len(targetFields) > 1 {
		return nil, fmt.Errorf("path %s returned %d items. Expected one", strings.Join(path, "."), len(targetFields))
	}

	target := targetFields[0]
	if target.YNode().Kind == yaml.ScalarNode {
		return []byte(target.YNode().Value), nil
	}

	return getJSONPayload(target)
}

func (e *jsonExtender) Set(path []string, value any) error {

	targetFields, err := Lookup(e.node, path)
	if err != nil {
		return fmt.Errorf("error fetching elements in replacement target: %w", err)
	}

	for _, t := range targetFields {
		if t.YNode().Kind == yaml.ScalarNode {
			t.YNode().Value = string(GetByteValue(value))
		} else {
			if v, ok := value.(*yaml.Node); ok {
				t.SetYNode(v)
			} else {
				return fmt.Errorf("setting non json object in place of object of type %s at path %s", t.YNode().Tag, strings.Join(path, "."))
			}
		}
	}
	return nil
}

func NewJsonExtender() Extender {
	return &jsonExtender{}
}

////////////
// Factories
////////////

var ExtenderFactories = map[ExtenderType]func() Extender{
	YamlExtender:   NewYamlExtender,
	Base64Extender: NewBase64Extender,
	RegexExtender:  NewRegexExtender,
	JsonExtender:   NewJsonExtender,
}

func (path *ExtendedSegment) Extender(payload []byte) (Extender, error) {
	bpt := GetExtenderType(cases.Title(language.English, cases.NoLower).String(path.Encoding) + "Extender")
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

type ExtendedPath struct {
	resourcePath     []string
	extendedSegments *[]*ExtendedSegment
}

func NewExtendedPath(path []string) (*ExtendedPath, error) {
	extensions := []*ExtendedSegment{}
	prefix, err := splitExtendedPath(path, &extensions)
	if err != nil {
		return nil, errors.WrapPrefixf(err, "while getting extended path")
	}

	return &ExtendedPath{resourcePath: prefix, extendedSegments: &extensions}, nil
}

func (ep *ExtendedPath) HasExtensions() bool {
	return len(*ep.extendedSegments) > 0
}

func (ep *ExtendedPath) String() string {
	out := strings.Join(ep.resourcePath, ".")
	if len(*ep.extendedSegments) > 0 {
		segmentStrings := []string{}
		for _, s := range *ep.extendedSegments {
			segmentStrings = append(segmentStrings, s.String())
		}
		out = fmt.Sprintf("%s.%s", out, strings.Join(segmentStrings, "."))
	}
	return out
}

func (ep *ExtendedPath) ApplyIndex(index int, input []byte, value *yaml.Node) ([]byte, error) {
	if index >= len(*ep.extendedSegments) || index < 0 {
		return nil, fmt.Errorf("invalid extended path index: %d", index)
	}

	segment := (*ep.extendedSegments)[index]
	extender, err := segment.Extender(input)
	if err != nil {
		return nil, errors.WrapPrefixf(err, "creating extender at index: %d", index)
	}

	if index == len(*ep.extendedSegments)-1 {
		err := extender.Set(segment.Path, value)
		if err != nil {
			return nil, errors.WrapPrefixf(err, "setting value on path %s", segment.String())
		}
	} else {
		nextInput, err := extender.Get(segment.Path)
		if err != nil {
			return nil, errors.WrapPrefixf(err, "getting value on path %s", segment.String())
		}
		newValue, err := ep.ApplyIndex(index+1, nextInput, value)
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

func (ep *ExtendedPath) Apply(target *yaml.RNode, value *yaml.RNode) error {
	if target.YNode().Kind != yaml.ScalarNode {
		return fmt.Errorf("extended path only works on scalar nodes")
	}

	outValue := value.YNode().Value
	if len(*ep.extendedSegments) > 0 {
		input := []byte(target.YNode().Value)
		output, err := ep.ApplyIndex(0, input, value.YNode())
		if err != nil {
			return errors.WrapPrefixf(err, "applying value on extended segment %s", ep.String())
		}

		outValue = string(output)
	}
	target.YNode().Value = outValue
	return nil
}
