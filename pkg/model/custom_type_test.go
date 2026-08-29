package model

import (
	"testing"

	"github.com/realmroot/lightkite/pkg/common"
)

func TestSliceString_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected SliceString
		wantErr  bool
	}{
		{"nil value", nil, nil, false},
		{"empty string", "", SliceString{}, false},
		{"JSON string", `["a","b,c"]`, SliceString{"a", "b,c"}, false},
		{"JSON bytes", []byte(`["x","y,z"]`), SliceString{"x", "y,z"}, false},
		{"legacy comma encoding", "a,b,c", nil, true},
		{"single legacy value", "single", nil, true},
		{"unsupported type", 123, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SliceString
			err := s.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !equalSliceString(s, tt.expected) {
				t.Errorf("Scan() got = %v, want %v", s, tt.expected)
			}
		})
	}
}

func TestSliceString_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    SliceString
		expected string
	}{
		{"nil slice", nil, "[]"},
		{"empty slice", SliceString{}, "[]"},
		{"single value", SliceString{"foo"}, `["foo"]`},
		{"multiple values preserve commas", SliceString{"a", "b,c"}, `["a","b,c"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()
			if err != nil {
				t.Errorf("Value() error = %v", err)
			}
			if val != tt.expected {
				t.Errorf("Value() got = %v, want %v", val, tt.expected)
			}
		})
	}
}

func TestSecretString_ScanAndValue(t *testing.T) {
	originalKey := common.LightkiteEncryptKey
	common.LightkiteEncryptKey = "test-encryption-key"
	t.Cleanup(func() {
		common.LightkiteEncryptKey = originalKey
	})

	secret := SecretString("super-secret")
	encoded, err := secret.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if encoded == "super-secret" || encoded == "" {
		t.Fatalf("Value() got unexpected encoded value = %v", encoded)
	}

	var decoded SecretString
	if err := decoded.Scan(encoded); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if decoded != secret {
		t.Fatalf("Scan() got = %v, want %v", decoded, secret)
	}
}

func equalSliceString(a, b SliceString) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
