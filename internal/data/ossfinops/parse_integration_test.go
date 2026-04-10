//go:build integration

package ossfinops

import (
	"os"
	"strings"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// TestOSSCSVRowCount 需 OSS_BILLING_BUCKET、OSS_*、ALIBABA_CLOUD_ACCESS_KEY_ID_UAT 等：go test -tags=integration -v -run TestOSSCSVRowCount
func TestOSSCSVRowCount(t *testing.T) {
	bucket := strings.TrimSpace(os.Getenv("OSS_BILLING_BUCKET"))
	if bucket == "" {
		t.Skip("no OSS_BILLING_BUCKET")
	}
	ak, sk := EnvForFinOps("UAT")
	if ak == "" || sk == "" {
		t.Skip("no UAT AK/SK")
	}
	endpoint := os.Getenv("OSS_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://oss-ap-southeast-1.aliyuncs.com"
	}
	cli, err := oss.New(endpoint, ak, sk)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cli.Bucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	key := "billing-data/1657988574642393-20260325130300_consumedetailbillv2_2026-01.csv"
	rc, err := b.GetObject(key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	rows, cycle, err := parseCSVToFacts(rc, key, "UAT", GuessBillingCycleFromKey(key))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("billing_cycle=%s rows=%d", cycle, len(rows))
	if len(rows) == 0 {
		t.Fatal("expected at least one row from 2026-01 consumeDetail CSV")
	}
}
