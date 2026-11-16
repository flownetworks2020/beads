package main

import (
	"strconv"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseIfRevisionPreservesSignedWidth(t *testing.T) {
	for _, raw := range []string{"-9223372036854775808", "9223372036854775807", "9007199254740993", "-9007199254740993", "0"} {
		t.Run(raw, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("if-revision", "", "")
			if err := cmd.Flags().Set("if-revision", raw); err != nil {
				t.Fatal(err)
			}
			got, err := parseIfRevisionFlag(cmd)
			if err != nil || got == nil || strconv.FormatInt(*got, 10) != raw {
				t.Fatalf("parse %q: %v, %v", raw, got, err)
			}
		})
	}
	for _, raw := range []string{"", "unknown", "1.0", "9223372036854775808", "-9223372036854775809"} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("if-revision", "", "")
			if err := cmd.Flags().Set("if-revision", raw); err != nil {
				t.Fatal(err)
			}
			if got, err := parseIfRevisionFlag(cmd); err == nil || got != nil {
				t.Fatalf("accepted invalid revision %q", raw)
			}
		})
	}
}
