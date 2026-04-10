// 从 OSS_BILLING_PREFIX 下取一个 .csv（可 -key 指定），打印首行表头并检测是否含抹零相关列。密钥不入日志。[Ref: 04_采集 §七]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/myxxhui/lighthouse-src/internal/data/ossfinops"
)

func main() {
	keyFlag := flag.String("key", "", "optional OSS object key; default: first billable object (.csv or BillingItemDetail) under prefix")
	flag.Parse()

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
	credEnv := strings.TrimSpace(os.Getenv("OSS_LIST_ENV"))
	if credEnv == "" {
		credEnv = strings.TrimSpace(os.Getenv("OSS_BILLING_CREDENTIAL_ENV"))
	}
	if credEnv == "" {
		credEnv = "C66_POC"
	}
	ak, sk := ossfinops.EnvForFinOps(credEnv)
	if ak == "" {
		ak, sk = ossfinops.EnvForFinOps("POC")
	}
	if ak == "" {
		ak = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
		sk = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	}
	if bucketName == "" || ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "need OSS_BILLING_BUCKET, OSS_BILLING_PREFIX, and AK/SK (e.g. OSS_LIST_ENV=C66_POC + ALIBABA_CLOUD_ACCESS_KEY_ID_C66_POC)")
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

	targetKey := strings.TrimSpace(*keyFlag)
	if targetKey == "" {
		targetKey, err = firstCSVKey(bucket, prefix)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if targetKey == "" {
		fmt.Fprintln(os.Stderr, "no billable CSV object under prefix")
		os.Exit(1)
	}

	body, err := bucket.GetObject(targetKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetObject:", err)
		os.Exit(1)
	}
	defer body.Close()

	limited := io.LimitReader(body, 64*1024)
	br := bufio.NewReader(limited)
	var header string
	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		s := strings.TrimSpace(line)
		if s != "" && !strings.HasPrefix(s, "#") {
			header = s
			break
		}
		if err == io.EOF {
			break
		}
	}

	fmt.Println("=== OSS CSV 抽样（首行非空表头）===")
	fmt.Printf("object: %s\n", targetKey)
	fmt.Printf("header: %s\n\n", header)

	low := strings.ToLower(header)
	hasMo := strings.Contains(header, "抹零") || strings.Contains(low, "round") || strings.Contains(low, "roundoff")
	fmt.Printf("抹零相关列（子串粗检）: %v\n", hasMo)
	if !hasMo {
		fmt.Println("说明: 表头中未出现「抹零」或 round/roundoff 等常见英文；若阿里云使用其它列名，需在 ossfinops/load.go findCol 中补别名。")
	}
}

func firstCSVKey(bucket *oss.Bucket, prefix string) (string, error) {
	objs, err := ossfinops.ListOSSBillingObjects(bucket, prefix)
	if err != nil {
		return "", err
	}
	if len(objs) == 0 {
		return "", nil
	}
	return objs[0].Key, nil
}
