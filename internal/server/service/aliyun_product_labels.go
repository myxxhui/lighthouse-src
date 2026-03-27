// [Ref: 03_Phase6/01_FinOps 云产品明细 UX] 产品列仅展示「产品码 · 中文简称」，不出现领域大类名。
package service

import "strings"

// aliyunProductZH 常见阿里云 ProductCode → 中文简称（可随账单扩展 product_category_mapping 后迭代）。
var aliyunProductZH = map[string]string{
	"ECS": "云服务器", "ACK": "容器服务", "ACS": "容器计算", "ECI": "弹性容器实例",
	"RDS": "关系型数据库", "POLARDB": "PolarDB", "REDIS": "Redis", "MONGODB": "MongoDB",
	"OSS": "对象存储", "NAS": "文件存储", "DISK": "云盘", "YUNDISK": "云盘",
	"SLB": "负载均衡", "ALB": "应用型负载均衡", "NLB": "网络型负载均衡", "EIP": "弹性公网IP",
	"VPC": "专有网络", "NAT": "NAT网关", "DNS": "云解析", "CDN": "CDN",
	"WAF": "Web应用防火墙", "SAS": "安全中心", "KMS": "密钥管理", "CAS": "证书服务",
	"MSE": "微服务引擎", "SERVICEMESH": "服务网格", "SFM": "Serverless", "FC": "函数计算",
	"ACR": "容器镜像服务", "SWAS": "轻量应用服务器", "EVENTBRIDGE": "事件总线",
	"LOG": "日志服务", "SLS": "日志服务", "MQ": "消息队列", "KAFKA": "消息队列 Kafka",
	"ES": "Elasticsearch", "GPDB": "分析型数据库", "CLICKHOUSE": "ClickHouse",
	"DATAWORKS": "DataWorks", "MAXCOMPUTE": "MaxCompute",
}

// FormatAliyunProductDisplayName 返回「CODE · 中文简称」；未知码仅返回大写 CODE。
func FormatAliyunProductDisplayName(code string) string {
	c := strings.TrimSpace(strings.ToUpper(code))
	if c == "" {
		return ""
	}
	if zh, ok := aliyunProductZH[c]; ok {
		return c + " · " + zh
	}
	return c
}

// invalidDrilldownProductCode 过滤领域汇总键误解析出的「假产品码」（如「计算资源」整段）。
func invalidDrilldownProductCode(code string) bool {
	c := strings.TrimSpace(code)
	if c == "" || strings.Contains(c, ":") {
		return true
	}
	for _, d := range []string{"计算资源", "存储", "网络", "安全", "其他", "其它"} {
		if c == d {
			return true
		}
	}
	return false
}

// upgradeDrilldownCategory 同一产品码在多领域键下重复出现时，取更高优先级分类（安全>存储>网络>计算）。
func upgradeDrilldownCategory(cur, add string) string {
	if add == "" {
		return cur
	}
	if cur == "" {
		return add
	}
	pri := map[string]int{"security": 4, "storage": 3, "network": 2, "compute": 1}
	if pri[add] > pri[cur] {
		return add
	}
	return cur
}
