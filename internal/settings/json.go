package settings

import (
	"encoding/json"
	"fmt"
)

func EncodeValues(values map[string]Value) (string, error) {
	reg := Default()
	out := make(map[string]json.RawMessage, len(values))
	for k, v := range values {
		s, ok := reg.Lookup(k)
		if !ok {
			continue
		}
		raw, err := encodeValue(s.Type, v)
		if err != nil {
			return "", fmt.Errorf("encode %s: %w", k, err)
		}
		out[k] = raw
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func DecodeValues(data string) (map[string]Value, error) {
	if data == "" || data == "null" {
		return map[string]Value{}, nil
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	reg := Default()
	out := make(map[string]Value, len(raw))
	for k, msg := range raw {
		s, ok := reg.Lookup(k)
		if !ok {
			continue
		}
		v, err := decodeValue(s.Type, msg)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}

func encodeValue(t Type, v Value) (json.RawMessage, error) {
	switch t {
	case TypeFloat:
		return json.Marshal(v.Float)
	case TypeInt:
		return json.Marshal(v.Int)
	case TypeBool:
		return json.Marshal(v.Bool)
	case TypeString:
		return json.Marshal(v.String)
	}
	return nil, fmt.Errorf("unknown type %d", t)
}

func decodeValue(t Type, msg json.RawMessage) (Value, error) {
	switch t {
	case TypeFloat:
		var f float64
		if err := json.Unmarshal(msg, &f); err != nil {
			return Value{}, err
		}
		return FloatVal(f), nil
	case TypeInt:
		var n int
		if err := json.Unmarshal(msg, &n); err != nil {
			return Value{}, err
		}
		return IntVal(n), nil
	case TypeBool:
		var b bool
		if err := json.Unmarshal(msg, &b); err != nil {
			return Value{}, err
		}
		return BoolVal(b), nil
	case TypeString:
		var s string
		if err := json.Unmarshal(msg, &s); err != nil {
			return Value{}, err
		}
		return StringVal(s), nil
	}
	return Value{}, fmt.Errorf("unknown type %d", t)
}
