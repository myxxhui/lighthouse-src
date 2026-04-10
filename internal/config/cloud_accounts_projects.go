// Package config — 项目级云账号 YAML（AK/SK 引用环境变量，不落盘明文）。[Ref: 03_Phase6 项目云账号]
package config

import (
	"strings"
)

// CloudAccountsProjectsFile 根文档；与 cost_env_account_config.environment 对齐的是 EnvironmentKey（如 C66_UAT）。
// 实际加载由 LoadLighthouseDeployYAML 解析同一文件后填充。[Ref: lighthouse-deploy/config/cloud-accounts-projects.yaml]
type CloudAccountsProjectsFile struct {
	Version  int                    `yaml:"version"`
	Projects []CloudAccountsProject `yaml:"projects"`
}

// CloudAccountsProject 业务项目（如 C66），其下多个云环境账号各有一套凭证与 OSS。
type CloudAccountsProject struct {
	ID           string                    `yaml:"id"`
	Environments []CloudAccountsProjectEnv `yaml:"environments"`
}

// CloudAccountsProjectEnv 单环境：账单 API + 可选账单 OSS；密钥字段支持 ${VAR} 引用进程环境变量。
type CloudAccountsProjectEnv struct {
	Name            string `yaml:"name"` // 展示用短名，如 UAT
	EnvironmentKey  string `yaml:"environment_key"` // 唯一键，与 DB cost_env_account_config.environment 一致；建议含项目标识（如 C66_UAT）；空则 id + "_" + name
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	BSSEndpoint     string `yaml:"bss_endpoint"` // 可选；空则 deployment 全局 CLOUD_BILLING_ENDPOINT 或中国站默认
	OSSBilling      *struct {
		Bucket   string `yaml:"bucket"`
		Prefix   string `yaml:"prefix"`
		Endpoint string `yaml:"endpoint"` // OSS API endpoint，如 https://oss-ap-southeast-1.aliyuncs.com
	} `yaml:"oss_billing"`
	FinOpsCGSource string `yaml:"finops_cg_source"` // 可选 oss|api，覆盖 FINOPS_CG_SOURCE_<ENV>
}

// LoadCloudAccountsProjects 读取统一部署 YAML 中的 projects 段；path 空则与 LoadLighthouseDeployYAML 相同候选路径。文件不存在返回 (nil, nil)。[Ref: 03_Phase6 项目云账号]
func LoadCloudAccountsProjects(path string) (*CloudAccountsProjectsFile, error) {
	doc, err := LoadLighthouseDeployYAML(path)
	if err != nil || doc == nil {
		return nil, err
	}
	return &CloudAccountsProjectsFile{Version: doc.Version, Projects: doc.Projects}, nil
}

// MergeFinOpsCGSourceFromProjects 将各 environment_key 的 finops_cg_source 写入 cfg.FinOpsCGSourceByEnv（大写键，覆盖同名）。须在 fillFinOpsFromEnv 之后调用，以便项目级覆盖环境变量。[Ref: 03_Phase6/01_FinOps]
func MergeFinOpsCGSourceFromProjects(doc *CloudAccountsProjectsFile, cfg *Config) {
	if doc == nil || cfg == nil {
		return
	}
	if cfg.FinOpsCGSourceByEnv == nil {
		cfg.FinOpsCGSourceByEnv = make(map[string]string)
	}
	for _, p := range doc.Projects {
		for _, e := range p.Environments {
			key := strings.ToUpper(strings.TrimSpace(e.EnvironmentKey))
			if key == "" {
				continue
			}
			if e.FinOpsCGSource != "" {
				cfg.FinOpsCGSourceByEnv[key] = EffectiveFinOpsCGSource(e.FinOpsCGSource)
			}
		}
	}
}
