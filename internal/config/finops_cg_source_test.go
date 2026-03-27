package config

import "testing"

func TestEffectiveFinOpsCGSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
	}{
		{"", "oss"},
		{"  ", "oss"},
		{"oss", "oss"},
		{"OSS", "oss"},
		{"api", "api"},
		{"API", "api"},
		{"bogus", "oss"},
	}
	for _, tc := range cases {
		if g := EffectiveFinOpsCGSource(tc.raw); g != tc.want {
			t.Fatalf("EffectiveFinOpsCGSource(%q)=%q want %q", tc.raw, g, tc.want)
		}
	}
}

func TestBuildFinOpsCGSourceByEnvMap(t *testing.T) {
	t.Parallel()
	m := BuildFinOpsCGSourceByEnvMap(map[string]string{"poc": "api", "UAT": "OSS"})
	if m["POC"] != "api" || m["UAT"] != "oss" {
		t.Fatalf("got %#v", m)
	}
	if BuildFinOpsCGSourceByEnvMap(nil) != nil {
		t.Fatal("nil in nil out")
	}
}

func TestParseFinOpsCGSourceByEnvFromEnviron(t *testing.T) {
	t.Parallel()
	m := parseFinOpsCGSourceByEnvFromEnviron([]string{
		"FINOPS_CG_SOURCE_POC=api",
		"FINOPS_CG_SOURCE_MY_ENV=OSS",
		"FINOPS_CG_SOURCE=",
		"OTHER=1",
		"PATH=/usr/bin",
	})
	if m["POC"] != "api" || m["MY_ENV"] != "oss" {
		t.Fatalf("got %#v", m)
	}
}

func TestMergeFinOpsCGFromEnvironSlice(t *testing.T) {
	t.Parallel()
	cfg := &Config{FinOpsCGSourceByEnv: map[string]string{"POC": "oss"}}
	mergeFinOpsCGFromEnvironSlice(cfg, []string{"FINOPS_CG_SOURCE_STAGING=api"})
	if cfg.FinOpsCGSourceByEnv["STAGING"] != "api" {
		t.Fatalf("STAGING=%q", cfg.FinOpsCGSourceByEnv["STAGING"])
	}
	if cfg.FinOpsCGSourceByEnv["POC"] != "oss" {
		t.Fatal("YAML POC should remain when not overridden")
	}
}
