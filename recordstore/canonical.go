// Package recordstore implements the inactive immutable storage foundation.
// Replay is pure and inactive; no runtime migration or adapter is enabled.
package recordstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Canonical returns ADR 0001's ASCII JSON encoding. It rejects ambiguous JSON
// before encoding/json can replace malformed Unicode or discard duplicate keys.
func Canonical(raw []byte) ([]byte, error) {
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("record exceeds size limit")
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid UTF-8 JSON")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		for i++; raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			n, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return nil, fmt.Errorf("unpaired low surrogate")
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return nil, fmt.Errorf("unpaired high surrogate")
				}
				low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err != nil || low < 0xdc00 || low > 0xdfff {
					return nil, fmt.Errorf("unpaired high surrogate")
				}
				i += 6
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	var out bytes.Buffer
	encode(&out, value)
	if out.Len() > MaxBytes {
		return nil, fmt.Errorf("canonical record exceeds size limit")
	}
	return out.Bytes(), nil
}

func readValue(d *json.Decoder) (any, error) { return readValueDepth(d, 0) }

func readValueDepth(d *json.Decoder, depth int) (any, error) {
	if depth >= 256 {
		return nil, fmt.Errorf("JSON nesting limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		if value == '{' {
			object := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, err
				}
				name := key.(string)
				if _, exists := object[name]; exists {
					return nil, fmt.Errorf("duplicate key %q", name)
				}
				child, err := readValueDepth(d, depth+1)
				if err != nil {
					return nil, err
				}
				object[name] = child
			}
			_, err = d.Token()
			return object, err
		}
		array := []any{}
		for d.More() {
			child, err := readValueDepth(d, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, child)
		}
		_, err = d.Token()
		return array, err
	case json.Number:
		if len(value) > 128 {
			return nil, fmt.Errorf("oversized number")
		}
		if parts := strings.FieldsFunc(string(value), func(c rune) bool { return c == 'e' || c == 'E' }); len(parts) == 2 {
			exponent, err := strconv.Atoi(parts[1])
			if err != nil || exponent < -1000 || exponent > 1000 {
				return nil, fmt.Errorf("oversized exponent")
			}
		}
		n, ok := new(big.Rat).SetString(string(value))
		if !ok || !n.IsInt() || !n.Num().IsInt64() || n.Num().Int64() < -9007199254740991 || n.Num().Int64() > 9007199254740991 {
			return nil, fmt.Errorf("number is not a safe integer")
		}
		return n.Num().Int64(), nil
	default:
		return token, nil
	}
}

func less(a, b string) bool {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b))) < 0
}

func encode(out *bytes.Buffer, value any) {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(v))
	case int64:
		out.WriteString(strconv.FormatInt(v, 10))
	case string:
		out.WriteByte('"')
		for _, c := range utf16.Encode([]rune(v)) {
			switch {
			case c == '"' || c == '\\':
				out.WriteByte('\\')
				out.WriteByte(byte(c))
			case c < 32 || c > 126:
				fmt.Fprintf(out, "\\u%04x", c)
			default:
				out.WriteByte(byte(c))
			}
		}
		out.WriteByte('"')
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			encode(out, item)
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) })
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			encode(out, key)
			out.WriteByte(':')
			encode(out, v[key])
		}
		out.WriteByte('}')
	}
}

func ordered(values []string) bool {
	for i := 1; i < len(values); i++ {
		if !less(values[i-1], values[i]) {
			return false
		}
	}
	return true
}

func validID(id string) bool {
	return len(id) == 64 && strings.IndexFunc(id, func(c rune) bool { return !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') }) == -1
}
