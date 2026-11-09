package main

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Stage order; "" is a final release.
var rank = map[string]int{"alpha": 0, "beta": 1, "rc": 2, "": 3}

// Version is X.Y.Z or X.Y.Z-stage.N, optionally prefixed with "v".
type Version struct {
	Prefix              string
	Major, Minor, Patch int
	Stage               string
	N                   int
}

func Parse(s string) (Version, bool) {

	var v Version
	if rest, ok := strings.CutPrefix(s, "v"); ok {
		v.Prefix, s = "v", rest
	}

	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}

	var ok [3]bool
	v.Major, ok[0] = num(parts[0])
	v.Minor, ok[1] = num(parts[1])
	v.Patch, ok[2] = num(parts[2])
	if !ok[0] || !ok[1] || !ok[2] {
		return v, false
	}

	if hasPre {
		stage, n, _ := strings.Cut(pre, ".")
		if r, known := rank[stage]; !known || r == rank[""] {
			return v, false
		}
		var valid bool
		v.Stage = stage
		v.N, valid = num(n)
		if !valid || v.N < 1 {
			return v, false
		}
	}

	return v, true
}

func (v Version) String() string {

	s := fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
	if v.Stage != "" {
		s += fmt.Sprintf("-%s.%d", v.Stage, v.N)
	}

	return s
}

// Compare orders by semver precedence, ignoring the prefix.
func Compare(a, b Version) int {

	return cmp.Or(
		cmp.Compare(a.Major, b.Major),
		cmp.Compare(a.Minor, b.Minor),
		cmp.Compare(a.Patch, b.Patch),
		cmp.Compare(rank[a.Stage], rank[b.Stage]),
		cmp.Compare(a.N, b.N),
	)
}

// Next resolves the tag to create. scope is "", "major", "minor" or "patch";
// stage is "" (auto), "alpha", "beta", "rc" or "final".
func Next(tags []Version, scope, stage string) (Version, error) {

	var latest, final Version
	for _, t := range tags {
		if Compare(t, latest) > 0 {
			latest = t
		}
		if t.Stage == "" && Compare(t, final) > 0 {
			final = t
		}
	}
	open := latest.Stage != ""

	var target Version
	switch {
	case scope != "":
		target = bump(final, scope)
	case stage == "":
		if open {
			target, stage = latest, latest.Stage
		} else {
			target, stage = bump(latest, "minor"), "beta"
		}
	case stage == "final" && !open:
		return Version{}, fmt.Errorf("no open pre-release line to finalize (latest is %s); use 'tt mi' or 'tt p' to tag a final directly", latest)
	case open:
		target = latest
	default:
		target = bump(latest, "minor")
	}

	target.Prefix = latest.Prefix
	target.Stage, target.N = "", 0
	if stage != "" && stage != "final" {
		target.Stage, target.N = stage, 1
		for _, t := range tags {
			if sameCore(t, target) && t.Stage == stage && t.N >= target.N {
				target.N = t.N + 1
			}
		}
	}

	return target, guard(tags, target)
}

// guard enforces "never back": target must outrank every tag on the same X.Y.Z.
func guard(tags []Version, target Version) error {

	var block *Version
	for i, t := range tags {
		if sameCore(t, target) && Compare(t, target) >= 0 && (block == nil || Compare(t, *block) > 0) {
			block = &tags[i]
		}
	}

	if block != nil {
		return fmt.Errorf("refusing %s: %s already exists, can't go back", target, block)
	}

	return nil
}

func bump(v Version, scope string) Version {

	switch scope {
	case "major":
		return Version{Prefix: v.Prefix, Major: v.Major + 1}
	case "minor":
		return Version{Prefix: v.Prefix, Major: v.Major, Minor: v.Minor + 1}
	default:
		return Version{Prefix: v.Prefix, Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
	}
}

func sameCore(a, b Version) bool {
	return a.Major == b.Major && a.Minor == b.Minor && a.Patch == b.Patch
}

func num(s string) (int, bool) {

	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)

	return n, err == nil
}

// sortDesc returns tags highest first.
func sortDesc(tags []Version) []Version {

	s := slices.Clone(tags)
	slices.SortFunc(s, func(a, b Version) int { return Compare(b, a) })

	return s
}
