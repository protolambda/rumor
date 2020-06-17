package flags

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// BytesHex exposes bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type BytesHexFlag []byte

func (f BytesHexFlag) String() string {
	return hex.EncodeToString(f)
}

func (f *BytesHexFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	*f = b
	return nil
}

func parseRoot(v string) ([32]byte, error) {
	if v == "0" {
		return [32]byte{}, nil
	}
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 64 {
		return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [32]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

func parseForkVersion(v string) ([4]byte, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 8 {
		return [4]byte{}, fmt.Errorf("provided fork version has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [4]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}
