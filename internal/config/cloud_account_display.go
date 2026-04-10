package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CloudAccountDisplay 云环境账户卡展示名（YAML）；与 DB cost_project 的 name、environment 对齐。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
// AK/SK 仅通过环境变量注入（如 ALIBABA_CLOUD_ACCESS_KEY_ID_POC），禁止在本文件写密钥。
type CloudAccountDisplay struct {
	Version  int                           `yaml:"version"`
	Projects []CloudAccountDisplayProject `yaml:"projects"`
}

// CloudAccountDisplayProject 项目名称与 cost_project.name 一致（大小写不敏感匹配）。
type CloudAccountDisplayProject struct {
	Name     string                        `yaml:"name"`
	Accounts []CloudAccountDisplayAccount `yaml:"accounts"`
}

// CloudAccountDisplayAccount 单环境账户展示；标题格式 {项目名}-{cloud}-{display_env}。
type CloudAccountDisplayAccount struct {
	Environment         string `yaml:"environment"`           // 与 cost_env_account_config.environment 一致，如 POC、UAT
	Cloud               string `yaml:"cloud"`                 // 如云 Aliyun（默认 Aliyun）
	Site                string `yaml:"site"`                  // domestic | international（展示为中文副标题）
	CredentialEnvSuffix string `yaml:"credential_env_suffix"` // 仅文档：对应 ALIBABA_*_<ENV> 后缀，勿写 AK/SK
	DisplayEnv          string `yaml:"display_env"`           // 可选；标题第三段，默认将 UAT→Uat
}

// LoadCloudAccountDisplay 读取 YAML；路径优先 CLOUD_ACCOUNT_DISPLAY_YAML，其次常见挂载路径；文件不存在返回 (nil, nil)。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
func LoadCloudAccountDisplay(path string) (*CloudAccountDisplay, error) {
	if strings.TrimSpace(path) == "" {
		path = strings.TrimSpace(os.Getenv("CLOUD_ACCOUNT_DISPLAY_YAML"))
	}
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates,
		"/app/config/cloud-account-display.yaml",
		"./config/cloud-account-display.yaml",
		"configs/cloud-account-display.yaml",
		"../lighthouse-deploy/config/cloud-account-display.yaml",
	)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var out CloudAccountDisplay
		if err := yaml.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, nil
}
