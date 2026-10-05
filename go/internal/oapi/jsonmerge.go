package oapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
)

// JSONMerge overlays patch onto data and returns the result. It backs the
// Merge* helpers oapi-codegen generates on union types.
//
// Keys in patch win; keys only in data are kept; where both hold a JSON object
// the merge recurses. Anything else — arrays, scalars, null — is replaced by
// the patch value. A nil or empty side is treated as an empty object, and if
// either side is valid JSON but not an object, patch replaces data wholesale.
//
// This replaces github.com/oapi-codegen/runtime.JSONMerge, which merges arrays
// element by element; this replaces the whole array, which is what Merge*
// callers expect and all the generated union schemas need.
func JSONMerge(data, patch json.RawMessage) (json.RawMessage, error) {
	dataObj, dataOK, err := decodeObject(data)
	if err != nil {
		return nil, fmt.Errorf("merging JSON: existing value: %w", err)
	}
	patchObj, patchOK, err := decodeObject(patch)
	if err != nil {
		return nil, fmt.Errorf("merging JSON: patch value: %w", err)
	}

	// Nothing to merge into, or nothing that can hold a merge.
	if !dataOK || !patchOK {
		if len(bytes.TrimSpace(patch)) == 0 {
			return data, nil
		}
		return patch, nil
	}

	merged, err := json.Marshal(mergeObjects(dataObj, patchObj))
	if err != nil {
		return nil, fmt.Errorf("merging JSON: %w", err)
	}
	return merged, nil
}

// mergeObjects merges patch into data, recursing where both hold an object.
func mergeObjects(data, patch map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(data)+len(patch))
	maps.Copy(out, data)
	for k, patchVal := range patch {
		dataVal, present := out[k]
		if !present {
			out[k] = patchVal
			continue
		}
		dataSub, dataOK, _ := decodeObject(dataVal)
		patchSub, patchOK, _ := decodeObject(patchVal)
		if dataOK && patchOK {
			if nested, err := json.Marshal(mergeObjects(dataSub, patchSub)); err == nil {
				out[k] = nested
				continue
			}
		}
		out[k] = patchVal
	}
	return out
}

// decodeObject reports whether raw is a JSON object and, if so, returns its
// members with their bytes untouched — decoding through interface{} would
// round large integers through float64.
//
// Empty input is an empty object. Valid JSON that is not an object returns
// ok=false with no error; only malformed JSON is an error.
func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, true, nil
	}
	if trimmed[0] != '{' {
		if !json.Valid(trimmed) {
			return nil, false, fmt.Errorf("invalid JSON")
		}
		return nil, false, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, false, err
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	return obj, true, nil
}
