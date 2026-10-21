package zentao

import (
	"encoding/json"
	"testing"
)

func TestExtractBugID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"markdown link", "admin编辑了Bug [#3::test_bug3](http://x/index.php?m=bug&f=view&id=3)", "3"},
		{"url only", "http://x/index.php?m=bug&f=view&id=42", "42"},
		{"url with ampersand", "http://x/index.php?a=1&id=99", "99"},
		{"no id", "nothing here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractBugID(tt.text); got != tt.want {
				t.Errorf("ExtractBugID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIDUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ID
		wantErr bool
	}{
		{name: "string", input: `"3"`, want: "3"},
		{name: "number", input: `3`, want: "3"},
		{name: "boolean", input: `true`, wantErr: true},
		{name: "null", input: `null`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id ID
			err := json.Unmarshal([]byte(tt.input), &id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UnmarshalJSON() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}
			if id != tt.want {
				t.Errorf("ID = %q, want %q", id, tt.want)
			}
		})
	}
}
