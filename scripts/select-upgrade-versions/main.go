// select-upgrade-versions keeps the smoke matrix on supported upgrade paths.
// A maintained release branch must not open databases created by newer releases.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"golang.org/x/mod/semver"
)

func selectVersions(tags []string, candidate string, count int) ([]string, error) {
	if !semver.IsValid(candidate) || count < 1 {
		return nil, fmt.Errorf("valid candidate and positive count required")
	}
	seen := make(map[string]bool)
	versions := make([]string, 0, len(tags))
	for _, tag := range tags {
		if semver.IsValid(tag) && semver.Compare(tag, candidate) <= 0 && !seen[tag] {
			seen[tag] = true
			versions = append(versions, tag)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if order := semver.Compare(versions[i], versions[j]); order != 0 {
			return order > 0
		}
		return versions[i] < versions[j]
	})
	if len(versions) == 0 {
		return nil, fmt.Errorf("no released upgrade sources at or below %s", candidate)
	}
	if len(versions) > count {
		versions = versions[:count]
	}
	return versions, nil
}

func main() {
	candidate := flag.String("candidate", "", "candidate semantic version including v prefix")
	count := flag.Int("count", 5, "maximum number of upgrade sources")
	flag.Parse()
	var tags []string
	err := json.NewDecoder(os.Stdin).Decode(&tags)
	if err == nil {
		var versions []string
		versions, err = selectVersions(tags, *candidate, *count)
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(versions)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
