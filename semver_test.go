package main

import (
	"slices"
	"strings"
	"testing"
)

// Real-world tag history, deliberately in lexical (wrong) order.
const history = `6.3.0 6.3.0-beta.1 6.3.0-beta.10 6.3.0-beta.2 6.3.0-beta.9 6.3.0-rc.1 6.3.0-rc.2
6.3.1 6.3.2 6.4.0 6.4.0-beta.1 6.4.1 6.4.2 6.4.2-beta.1 6.5.0 6.5.0-beta.1 6.6.0 6.6.0-beta.1
6.6.0-beta.2 6.6.1 6.6.1-beta.1 6.7.0-beta.1 6.7.0-beta.2`

func parseAll(t *testing.T, s string) []Version {

	t.Helper()

	var vs []Version
	for _, f := range strings.Fields(s) {
		v, ok := Parse(f)
		if !ok {
			t.Fatalf("failed to parse %q", f)
		}
		vs = append(vs, v)
	}

	return vs
}

func TestParse(t *testing.T) {

	valid := map[string]Version{
		"1.2.3":         {Major: 1, Minor: 2, Patch: 3},
		"v1.2.3":        {Prefix: "v", Major: 1, Minor: 2, Patch: 3},
		"6.3.0-beta.10": {Major: 6, Minor: 3, Stage: "beta", N: 10},
		"v2.0.0-rc.1":   {Prefix: "v", Major: 2, Stage: "rc", N: 1},
		"0.1.0-alpha.3": {Minor: 1, Stage: "alpha", N: 3},
	}
	for in, want := range valid {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", in, got, ok, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q; want %q", got.String(), in)
		}
	}

	for _, in := range []string{"", "1.2", "1.2.3.4", "x1.2.3", "1.2.3-beta", "1.2.3-beta.0", "1.2.3-snapshot.1", "1.2.3-beta.x", "release-1"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestSort(t *testing.T) {

	vs := parseAll(t, "6.3.0 6.3.0-beta.10 6.3.0-rc.1 6.3.0-beta.2 6.2.9 6.3.0-alpha.5 6.10.0-beta.1")
	slices.SortFunc(vs, Compare)

	var got []string
	for _, v := range vs {
		got = append(got, v.String())
	}

	want := "6.2.9 6.3.0-alpha.5 6.3.0-beta.2 6.3.0-beta.10 6.3.0-rc.1 6.3.0 6.10.0-beta.1"
	if strings.Join(got, " ") != want {
		t.Errorf("sorted = %v\nwant     %v", got, want)
	}
}

func TestNext(t *testing.T) {

	open := parseAll(t, history)                       // latest 6.7.0-beta.2
	closed := parseAll(t, history+" 6.7.0-rc.1 6.7.0") // latest 6.7.0 final
	rcOpen := parseAll(t, history+" 6.7.0-rc.1")
	prefixed := parseAll(t, "v1.0.0 v1.1.0-beta.1")

	cases := []struct {
		name         string
		tags         []Version
		scope, stage string
		want         string
		wantErr      string
	}{
		{"auto continues open line", open, "", "", "6.7.0-beta.3", ""},
		{"auto after final bumps minor beta", closed, "", "", "6.8.0-beta.1", ""},
		{"auto continues rc", rcOpen, "", "", "6.7.0-rc.2", ""},
		{"auto with no tags", nil, "", "", "0.1.0-beta.1", ""},
		{"beta continues open line", open, "", "beta", "6.7.0-beta.3", ""},
		{"rc on open line resets counter", open, "", "rc", "6.7.0-rc.1", ""},
		{"final closes open line", open, "", "final", "6.7.0", ""},
		{"beta after final bumps minor", closed, "", "beta", "6.8.0-beta.1", ""},
		{"patch beta from latest final", open, "patch", "beta", "6.6.2-beta.1", ""},
		{"patch final hotfix while line open", open, "patch", "", "6.6.2", ""},
		{"patch after closed line", closed, "patch", "", "6.7.1", ""},
		{"major beta", open, "major", "beta", "7.0.0-beta.1", ""},
		{"minor beta joins open line", open, "minor", "beta", "6.7.0-beta.3", ""},
		{"never back: beta after rc", rcOpen, "", "beta", "", "6.7.0-rc.1"},
		{"never back: alpha after beta", open, "", "alpha", "", "6.7.0-beta.2"},
		{"final without open line refused", closed, "", "final", "", "no open pre-release"},
		{"keeps v prefix", prefixed, "", "", "v1.1.0-beta.2", ""},
		{"keeps v prefix on bump", prefixed, "major", "", "v2.0.0", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Next(c.tags, c.scope, c.stage)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v; want containing %q (got %s)", err, c.wantErr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.String() != c.want {
				t.Errorf("got %s; want %s", got, c.want)
			}
		})
	}
}

func TestGuard(t *testing.T) {

	tags := parseAll(t, "6.6.1 6.7.0-beta.4 6.7.0")
	for _, bad := range []string{"6.7.0-rc.1", "6.7.0-beta.5", "6.7.0", "v6.7.0"} {
		v, _ := Parse(bad)
		if err := guard(tags, v); err == nil {
			t.Errorf("guard should refuse %s", bad)
		}
	}
	for _, ok := range []string{"6.6.2", "6.7.1-beta.1", "6.8.0"} {
		v, _ := Parse(ok)
		if err := guard(tags, v); err != nil {
			t.Errorf("guard refused %s: %v", ok, err)
		}
	}
}
