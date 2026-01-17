package contextview

import (
	"testing"
)

func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantKey string
		wantVal string
		wantOk  bool
	}{
		{
			name:    "simple key=value",
			input:   "foo=bar",
			wantKey: "foo",
			wantVal: "bar",
			wantOk:  true,
		},
		{
			name:    "double quoted value",
			input:   `foo="hello world"`,
			wantKey: "foo",
			wantVal: "hello world",
			wantOk:  true,
		},
		{
			name:    "single quoted value",
			input:   `foo='hello world'`,
			wantKey: "foo",
			wantVal: "hello world",
			wantOk:  true,
		},
		{
			name:    "backtick quoted value",
			input:   "foo=`hello world`",
			wantKey: "foo",
			wantVal: "hello world",
			wantOk:  true,
		},
		{
			name:    "quoted sentence",
			input:   `TARGET="production cluster in us-east-1"`,
			wantKey: "TARGET",
			wantVal: "production cluster in us-east-1",
			wantOk:  true,
		},
		{
			name:    "value with equals",
			input:   `CONNECT_STRING="user=admin host=localhost"`,
			wantKey: "CONNECT_STRING",
			wantVal: "user=admin host=localhost",
			wantOk:  true,
		},
		{
			name:   "no equals sign",
			input:  "justkey",
			wantOk: false,
		},
		{
			name:   "empty key",
			input:  "=value",
			wantOk: false,
		},
		{
			name:    "empty value",
			input:   "key=",
			wantKey: "key",
			wantVal: "",
			wantOk:  true,
		},
		{
			name:    "unquoted with spaces",
			input:   "key=hello world",
			wantKey: "key",
			wantVal: "hello world",
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val, ok := parseKeyValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseKeyValue(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && key != tt.wantKey {
				t.Errorf("parseKeyValue(%q) key = %q, want %q", tt.input, key, tt.wantKey)
			}
			if ok && val != tt.wantVal {
				t.Errorf("parseKeyValue(%q) val = %q, want %q", tt.input, val, tt.wantVal)
			}
		})
	}
}
