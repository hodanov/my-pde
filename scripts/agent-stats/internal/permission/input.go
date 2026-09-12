package permission

import (
	"bytes"
	"encoding/json"
	"reflect"
)

func decodeInput(raw json.RawMessage) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

func canonicalInput(raw json.RawMessage) string {
	v, ok := decodeInput(raw)
	if !ok {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// inputMatches reports whether a transcript call's input carries every field
// the hook recorded, with equal values. The hook drops file bodies before
// recording, so the recorded input may be a subset of the call's.
func inputMatches(call, recorded json.RawMessage) bool {
	want, ok := decodeInput(recorded)
	if !ok {
		return false
	}
	got, ok := decodeInput(call)
	if !ok {
		return false
	}
	wantFields, isObject := want.(map[string]any)
	if !isObject {
		return reflect.DeepEqual(want, got)
	}
	gotFields, isObject := got.(map[string]any)
	if !isObject {
		return false
	}
	for key, value := range wantFields {
		if gotValue, ok := gotFields[key]; !ok || !reflect.DeepEqual(value, gotValue) {
			return false
		}
	}
	return true
}
