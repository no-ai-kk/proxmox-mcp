package main

import (
	"strings"
	"testing"
)

func TestParseAllowedCloneSource(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
		err   string
	}{
		{name: "unset", want: 0},
		{name: "valid", value: "901", want: 901},
		{name: "zero", value: "0", err: "positive decimal VMID"},
		{name: "negative", value: "-1", err: "positive decimal VMID"},
		{name: "non numeric", value: "901x", err: "positive decimal VMID"},
		{name: "whitespace is malformed", value: " 901", err: "positive decimal VMID"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAllowedCloneSource(tc.value)
			if tc.err == "" {
				if err != nil || got != tc.want {
					t.Fatalf("got %d, %v; want %d, nil", got, err, tc.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("got %v; want error containing %q", err, tc.err)
			}
		})
	}
}
