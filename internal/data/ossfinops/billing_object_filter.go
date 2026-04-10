// [Ref: 04_采集与ETL §七 R9、03_Phase6/01_FinOps 采集与 ETL]
package ossfinops

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// IsLikelyOSSBillingDataObjectKey 判断 OSS 对象是否应参与 FinOps 账单列举与摄取。
// BSS 订阅：无后缀的 BillingItemDetail、扁平 *.csv，以及 **内嵌 CSV 的 BillingItemDetail*.zip**（子目录常见）。[Ref: 04_采集 §七 R9 R13]
func IsLikelyOSSBillingDataObjectKey(key string) bool {
	lk := strings.ToLower(key)
	if strings.HasSuffix(lk, ".csv") {
		return true
	}
	b := strings.ToLower(baseName(key))
	return strings.Contains(b, "billingitemdetail")
}

// ListOSSBillingObjects 分页列举 prefix 下待摄取对象（与 LoadBillingCSVsFromOSS / SumCashPaymentFromOSS 一致）。
func ListOSSBillingObjects(bucket *oss.Bucket, prefix string) ([]oss.ObjectProperties, error) {
	var all []oss.ObjectProperties
	marker := ""
	for {
		lsRes, err := bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(200))
		if err != nil {
			return nil, fmt.Errorf("ListObjects: %w", err)
		}
		for _, obj := range lsRes.Objects {
			key := obj.Key
			if !IsLikelyOSSBillingDataObjectKey(key) {
				continue
			}
			if obj.Size == 0 {
				slog.Warn("ossfinops: skip zero-byte billing object", "key", key)
				continue
			}
			all = append(all, obj)
		}
		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
		if marker == "" {
			break
		}
	}
	return all, nil
}
