package extras

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// EncodingType enumerates the existing encoding types.
//
//go:generate go run golang.org/x/tools/cmd/stringer -type=EncodingType
type EncodingType int

const (
	UnknownEncoding EncodingType = iota
	Base64Encoding
	BCryptEncoding
	HexEncoding
)

var stringToEncodingTypeMap map[string]EncodingType

func init() { //nolint:gochecknoinits
	stringToEncodingTypeMap = makeStringToEncodingTypeMap()
}

// makeStringToEncodingTypeMap makes a map to get the appropriate
// [EncodingType] given its name.
func makeStringToEncodingTypeMap() (result map[string]EncodingType) {
	result = make(map[string]EncodingType, 3)
	for k := range EncoderFactories {
		result[strings.Replace(strings.ToLower(k.String()), "encoding", "", 1)] = k
	}
	return
}

// getEncodingType returns the appropriate [EncodingType] for the passed
// encoding type name
func getEncodingType(n string) EncodingType {
	result, ok := stringToEncodingTypeMap[strings.ToLower(n)]
	if ok {
		return result
	}
	return UnknownEncoding
}

// Encoder is an encoder function
type Encoder func(value string) (string, error)

// EncodeBase64 encodes value in base64
func EncodeBase64(value string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(value)), nil
}

// EncodeBcrypt generates the bcrypt hash of value.
func EncodeBcrypt(value string) (string, error) {
	encoded, err := bcrypt.GenerateFromPassword([]byte(value), 10)

	return string(encoded), err
}

// EncodeHex returns the hex string of value
func EncodeHex(value string) (string, error) {
	return hex.EncodeToString([]byte(value)), nil
}

// EncoderFactories register the [Encoder] factory functions for each
// [EncoderType].
var EncoderFactories = map[EncodingType]Encoder{
	Base64Encoding: EncodeBase64,
	BCryptEncoding: EncodeBcrypt,
	HexEncoding:    EncodeHex,
}

func GetEncodedValue(value string, encoding string) (string, error) {
	et := getEncodingType(encoding)
	if f, ok := EncoderFactories[et]; ok {
		return f(value)
	}

	return "", fmt.Errorf("encoding %s is unknown (%d)", encoding, et)
}
