package main

import (
	"reflect"
	"testing"
)

func TestSelectVersions(t *testing.T) {
	tags := []string{"v1.10.0", "v1.9.0", "v1.4.0-rc.2", "v1.3.3", "v1.3.4-rc.1", "v1.3.4", "v1.2.9", "v1.3.4", "nightly"}
	for _, tt := range []struct {
		candidate string
		count     int
		want      []string
	}{
		{"v1.3.4", 3, []string{"v1.3.4", "v1.3.4-rc.1", "v1.3.3"}},
		{"v1.3.4-rc.1", 2, []string{"v1.3.4-rc.1", "v1.3.3"}},
		{"v1.10.0", 2, []string{"v1.10.0", "v1.9.0"}},
	} {
		got, err := selectVersions(tags, tt.candidate, tt.count)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s: got %v, %v; want %v", tt.candidate, got, err, tt.want)
		}
	}
}

func TestSelectVersionsFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		candidate string
		count     int
	}{{"garbage", 5}, {"v1.3.4", 0}, {"v1.3.4", -1}, {"v0.1.0", 5}} {
		if _, err := selectVersions([]string{"v1.3.4"}, tt.candidate, tt.count); err == nil {
			t.Fatalf("expected failure for %+v", tt)
		}
	}
}
