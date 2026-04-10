// [Ref: 03_Phase6 项目云账号 YAML] 显式凭证与 OSS；与扁平 ALIBABA_* 环境变量模式互斥于同一 Worker。
package etl

// ProjectCloudProfile 来自 config/cloud-accounts-projects.yaml；非 nil 且 OSSBucket 非空时，
// SyncFinOps 中 OSS 段使用该桶与同一 RAM（AccessKeyID/Secret）连接 OSS，不读全局 OSS_BILLING_*。
type ProjectCloudProfile struct {
	ProjectID       string
	EnvironmentKey  string
	AccessKeyID     string
	AccessKeySecret string
	OSSBucket       string
	OSSPrefix       string
	OSSEndpoint     string // 如 https://oss-ap-southeast-1.aliyuncs.com
}
