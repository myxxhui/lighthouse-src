package config

import (
	"testing"
)

func TestMergeFinOpsCGSourceFromProjects(t *testing.T) {
	doc := &CloudAccountsProjectsFile{
		Projects: []CloudAccountsProject{
			{
				ID: "C66",
				Environments: []CloudAccountsProjectEnv{
					{EnvironmentKey: "C66_UAT", FinOpsCGSource: "oss"},
				},
			},
		},
	}
	cfg := &Config{}
	MergeFinOpsCGSourceFromProjects(doc, cfg)
	if cfg.FinOpsCGSourceByEnv["C66_UAT"] != "oss" {
		t.Fatalf("want oss override, got %q", cfg.FinOpsCGSourceByEnv["C66_UAT"])
	}
}

func TestExpandCloudAccountsProjectsEnv(t *testing.T) {
	t.Setenv("TEST_C66_AK", "ak-from-env")
	doc := &CloudAccountsProjectsFile{
		Projects: []CloudAccountsProject{
			{
				ID: "C66",
				Environments: []CloudAccountsProjectEnv{
					{
						Name:            "UAT",
						EnvironmentKey:  "C66_UAT",
						AccessKeyID:     "${TEST_C66_AK}",
						AccessKeySecret: "secret",
					},
				},
			},
		},
	}
	ExpandCloudAccountsProjectsEnv(doc)
	if doc.Projects[0].Environments[0].AccessKeyID != "ak-from-env" {
		t.Fatalf("expand: %q", doc.Projects[0].Environments[0].AccessKeyID)
	}
}
