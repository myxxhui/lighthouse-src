// 一次性工具：连接 OSS，列出 prefix 下账单对象（含无后缀 BillingItemDetail）并打印每个文件首行表头（不写库）。[Ref: Phase6 CSV 表头对齐、04_采集 §七 R9]
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

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
	bucketName := os.Getenv("OSS_BILLING_BUCKET")
	prefix := os.Getenv("OSS_BILLING_PREFIX")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	ak := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID_UAT")
	sk := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET_UAT")
	if ak == "" {
		ak = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
		sk = os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	}
	if bucketName == "" || ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "need OSS_BILLING_BUCKET and ALIBABA_CLOUD_ACCESS_KEY_ID_UAT (or unsuffixed) + SECRET")
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
		rc, err := bucket.GetObject(key)
		if err != nil {
			fmt.Printf("ERR %s: %v\n", key, err)
			continue
		}
		line, err := readFirstNonEmptyLine(rc)
		rc.Close()
		if err != nil {
			fmt.Printf("ERR %s read: %v\n", key, err)
			continue
		}
		fmt.Printf("=== %s ===\n%s\n\n", key, line)
	}
}

func readFirstNonEmptyLine(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	b, err := br.Peek(3)
	if err != nil && err != io.EOF {
		return "", err
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = br.Discard(3)
	}
	for {
		s, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		if len(s) > 0 && s[len(s)-1] == '\n' {
			s = s[:len(s)-1]
		}
		if len(s) > 0 && s[len(s)-1] == '\r' {
			s = s[:len(s)-1]
		}
		if strings.TrimSpace(s) != "" {
			return s, nil
		}
		if err == io.EOF {
			return "", io.ErrUnexpectedEOF
		}
	}
}
