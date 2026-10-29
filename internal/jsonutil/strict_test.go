package jsonutil

import (
	"strings"
	"testing"
)

type inner struct {
	Value int `json:"value"`
}

type document struct {
	Name     string            `json:"name"`
	Optional *string           `json:"optional,omitempty"`
	Items    []inner           `json:"items"`
	Labels   map[string]inner  `json:"labels"`
	Skipped  string            `json:"-"`
	Extra    map[string]string `json:"extra,omitempty"`
}

func TestDecodeRejectsAmbiguousDocuments(t *testing.T) {
	for _, row := range []struct {
		name, body string
		err        string // substring; empty means valid
	}{
		{name: "valid", body: `{"name":"a","items":[{"value":1}],"labels":{"x":{"value":2}}}`},
		{name: "duplicate key", body: `{"name":"a","name":"b"}`, err: "invalid JSON document"},
		{name: "nested duplicate key", body: `{"items":[{"value":1,"value":2}]}`, err: "invalid JSON document"},
		{name: "case-folded key", body: `{"Name":"a"}`, err: "unknown"},
		{name: "unknown field", body: `{"name":"a","other":1}`, err: "unknown"},
		{name: "unknown nested field", body: `{"items":[{"value":1,"other":2}]}`, err: "unknown"},
		{name: "unknown field in map value", body: `{"labels":{"x":{"Value":2}}}`, err: "unknown"},
		{name: "ignored field name", body: `{"Skipped":"x"}`, err: "unknown"},
		{name: "trailing data", body: `{"name":"a"} {}`, err: "trailing"},
		{name: "truncated", body: `{"name":`, err: "invalid JSON document"},
		{name: "invalid UTF-8", body: "{\"name\":\"\xff\"}", err: "UTF-8"},
		{name: "wrong type", body: `{"name":1}`, err: "invalid JSON fields"},
		{name: "too deep", body: strings.Repeat("[", 70) + strings.Repeat("]", 70), err: "invalid"},
	} {
		t.Run(row.name, func(t *testing.T) {
			var out document
			err := Decode([]byte(row.body), &out)
			switch {
			case row.err == "" && err != nil:
				t.Fatalf("valid document rejected: %v", err)
			case row.err != "" && (err == nil || !strings.Contains(err.Error(), row.err)):
				t.Fatalf("error %v, want %q", err, row.err)
			}
		})
	}
}

func TestDecodeErrorsNeverEchoInput(t *testing.T) {
	const secret = "token-value-3f1c"
	for _, body := range []string{
		`{"name":"` + secret + `","name":"x"}`,
		`{"` + secret + `":1}`,
		`{"name":` + secret + `}`,
	} {
		var out document
		err := Decode([]byte(body), &out)
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error echoes input: %v", err)
		}
	}
}

func TestRequiredDemandsNonOptionalFields(t *testing.T) {
	for _, row := range []struct {
		name, body string
		ok         bool
	}{
		{name: "complete", body: `{"name":"a","optional":"b","items":[{"value":1}],"labels":{"x":{"value":2}}}`, ok: true},
		{name: "optional omitted", body: `{"name":"a","items":[],"labels":{}}`, ok: true},
		{name: "nullable slice and map", body: `{"name":"a","items":null,"labels":null}`, ok: true},
		{name: "missing name", body: `{"items":[],"labels":{}}`},
		{name: "missing nested value", body: `{"name":"a","items":[{}],"labels":{}}`},
		{name: "null scalar", body: `{"name":null,"items":[],"labels":{}}`},
		{name: "missing in map value", body: `{"name":"a","items":[],"labels":{"x":{}}}`},
		{name: "not an object", body: `[]`},
	} {
		t.Run(row.name, func(t *testing.T) {
			err := Required([]byte(row.body), &document{})
			if row.ok != (err == nil) {
				t.Fatalf("Required(%s) = %v", row.body, err)
			}
		})
	}
}
