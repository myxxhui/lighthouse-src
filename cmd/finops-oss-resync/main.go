// 手工触发 OSS→finops_billing_fact 全量同步（与 ETL RunPipeline ⓪ 步 OSS 段一致）。
// 使用项目 YAML 时须指定 -environment-key=C66_POC（与 Worker EnvKey 一致），否则会走全局 OSS_BILLING_*，与 compose 挂载的 cloud-accounts-projects.yaml 不一致。
//
//	cd lighthouse-deploy && set -a && source .env && set +a && cd ../lighthouse-src
//	OSS_FULL_SYNC=1 go run ./cmd/finops-oss-resync -environment-key=C66_POC
//
// [Ref: 04_采集 §七 R10]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/config"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/worker/etl"
)

func main() {
	envKey := flag.String("env", "POC", "与 cost_env_account_config.environment 一致（短名，如 POC）；-environment-key 未设时使用")
	environmentKey := flag.String("environment-key", "", "YAML 的 environment_key，如 C66_POC；设后优先用于 OSS profile 与 Worker 键")
	accountID := flag.String("account-id", "", "落库 finops_billing_fact.account_id；空则从 cost_env_account_config 读取")
	flag.Parse()

	cfg := &config.Config{}
	fillPostgresFromEnv(cfg)
	if cfg.Postgres.Host == "" {
		fmt.Fprintln(os.Stderr, "finops-oss-resync: 需要 PG_HOST 或 POSTGRES_HOST")
		os.Exit(1)
	}

	repo, err := postgres.NewPGRepository(cfg.Postgres)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer repo.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	workerKey := strings.TrimSpace(*environmentKey)
	if workerKey == "" {
		workerKey = strings.TrimSpace(*envKey)
	} else {
		workerKey = strings.ToUpper(workerKey)
	}

	aid := strings.TrimSpace(*accountID)
	if aid == "" {
		aid, err = lookupAccountIDFlexible(ctx, repo, workerKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lookup account_id: %v\n", err)
			os.Exit(1)
		}
	}
	if aid == "" {
		aid = workerKey
		fmt.Fprintf(os.Stderr, "warn: no account_id in cost_env_account_config for %s, using key as account_id\n", workerKey)
	}

	var prof *etl.ProjectCloudProfile
	if doc, derr := config.LoadLighthouseDeployYAML(""); derr == nil && doc != nil {
		prof = projectProfileForEnvKey(doc, workerKey)
	}

	fmt.Printf("finops-oss-resync: worker_key=%s account_id=%s yaml_profile=%v\n", workerKey, aid, prof != nil && strings.TrimSpace(prof.OSSBucket) != "")
	if err := etl.RunOSSBillingSyncFromEnv(ctx, repo, workerKey, aid, prof); err != nil {
		fmt.Fprintf(os.Stderr, "OSS sync failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("finops-oss-resync: ok")
}

func projectProfileForEnvKey(doc *config.LighthouseDeployYAML, wantKey string) *etl.ProjectCloudProfile {
	if doc == nil {
		return nil
	}
	wantKey = strings.ToUpper(strings.TrimSpace(wantKey))
	for i := range doc.Projects {
		p := &doc.Projects[i]
		for j := range p.Environments {
			e := &p.Environments[j]
			k := strings.ToUpper(strings.TrimSpace(e.EnvironmentKey))
			if k != wantKey {
				continue
			}
			prof := &etl.ProjectCloudProfile{
				ProjectID:       strings.TrimSpace(p.ID),
				EnvironmentKey:  k,
				AccessKeyID:     strings.TrimSpace(e.AccessKeyID),
				AccessKeySecret: strings.TrimSpace(e.AccessKeySecret),
			}
			if e.OSSBilling != nil {
				prof.OSSBucket = strings.TrimSpace(e.OSSBilling.Bucket)
				prof.OSSPrefix = strings.TrimSpace(e.OSSBilling.Prefix)
				prof.OSSEndpoint = strings.TrimSpace(e.OSSBilling.Endpoint)
			}
			return prof
		}
	}
	return nil
}

// lookupAccountIDFlexible 匹配 environment；YAML 键 C66_POC 时回退匹配 POC（与种子 cost_env_account_config 兼容）。[Ref: 03_Phase6 项目云账号]
func lookupAccountIDFlexible(ctx context.Context, repo *postgres.PGRepository, key string) (string, error) {
	list, err := repo.ListEnvAccountConfig(ctx)
	if err != nil {
		return "", err
	}
	key = strings.TrimSpace(key)
	for _, c := range list {
		if strings.EqualFold(strings.TrimSpace(c.Environment), key) {
			return strings.TrimSpace(c.AccountID), nil
		}
	}
	if i := strings.LastIndex(key, "_"); i > 0 {
		short := key[i+1:]
		for _, c := range list {
			if strings.EqualFold(strings.TrimSpace(c.Environment), short) {
				return strings.TrimSpace(c.AccountID), nil
			}
		}
	}
	return "", nil
}

func fillPostgresFromEnv(cfg *config.Config) {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	if v := get("PG_HOST", "POSTGRES_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := get("PG_PORT", "POSTGRES_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Postgres.Port = p
		}
	}
	if v := get("PG_USER", "POSTGRES_USER"); v != "" {
		cfg.Postgres.User = v
	}
	if v := get("PG_PASSWORD", "POSTGRES_PASSWORD"); v != "" {
		cfg.Postgres.Password = v
	}
	if v := get("PG_DATABASE", "POSTGRES_DB"); v != "" {
		cfg.Postgres.Database = v
	}
	if v := get("PG_SSL_MODE"); v != "" {
		cfg.Postgres.SSLMode = v
	}
}
