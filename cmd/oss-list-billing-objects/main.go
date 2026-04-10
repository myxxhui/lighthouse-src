// 列出 OSS_BILLING_PREFIX 下全部账单对象（.csv 及无后缀 BillingItemDetail），并打印 GuessBillingCycleFromKey 结果。用于排查某账期未入库。[Ref: 04_采集 §七 R9]
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/myxxhui/lighthouse-src/internal/data/ossfinops"
)

func main() {
	endpoint := os.Getenv("OSS_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
	}
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}
	bucketName := strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET"))
	prefix := strings.TrimSpace(os.Getenv("OSS_BILLING_PREFIX"))
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	// 与 syncOSS 一致：EnvForFinOps(env)；实践中 UAT AK 常被授予跨账号 bucket 读权限，POC 可能 403
	ak, sk := ossfinops.EnvForFinOps(strings.TrimSpace(os.Getenv("OSS_LIST_ENV")))
	if ak == "" {
		ak, sk = ossfinops.EnvForFinOps("UAT")
	}
	if ak == "" {
		ak, sk = ossfinops.EnvForFinOps("POC")
	}
	if ak == "" {
		ak = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
		sk = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	}
	if bucketName == "" || ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "need OSS_BILLING_BUCKET and AK/SK (POC or UAT or unsuffixed)")
		os.Exit(1)
	}
	cli, err := oss.New(endpoint, ak, sk)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bucket, err := cli.Bucket(bucketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	objs, err := ossfinops.ListOSSBillingObjects(bucket, prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, obj := range objs {
		key := obj.Key
		base := key
		if i := strings.LastIndex(key, "/"); i >= 0 {
			base = key[i+1:]
		}
		cycle := ossfinops.GuessBillingCycleFromKey(key)
		fmt.Printf("%s\t%s\t%d\t%s\n", obj.LastModified.UTC().Format(time.RFC3339), cycle, obj.Size, base)
	}
	fmt.Fprintf(os.Stderr, "billing_objects=%d\n", len(objs))
}
