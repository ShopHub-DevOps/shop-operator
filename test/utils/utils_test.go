package utils

import (
	"reflect"
	"testing"
)

func TestGetNonEmptyLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string", "", nil},
		{"single line without newline", "hello", []string{"hello"}},
		{"single line with trailing newline", "hello\n", []string{"hello"}},
		{"two lines", "alpha\nbeta", []string{"alpha", "beta"}},
		{"blank lines in between are dropped", "alpha\n\nbeta\n\n", []string{"alpha", "beta"}},
		{"only newlines yields no lines", "\n\n\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetNonEmptyLines(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("GetNonEmptyLines(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
