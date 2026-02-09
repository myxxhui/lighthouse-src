package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Loader 配置加载器接口
type Loader interface {
	Load() (*Config, error)
	Watch(stopChan <-chan struct{}) error
}

// FileLoader 文件配置加载器实现
type FileLoader struct {
	configPath string
	viper      *viper.Viper
}

func NewFileLoader(configPath string) Loader {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)

	return &FileLoader{
		configPath: configPath,
		viper:      v,
	}
}

func (l *FileLoader) Load() (*Config, error) {
	// 加载基础配置
	if err := l.viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// 加载环境特定配置
	env := os.Getenv("ENV")
	if env != "" {
		envConfig := fmt.Sprintf("config.%s", env)
		l.viper.SetConfigName(envConfig)
		if err := l.viper.MergeInConfig(); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to merge %s config: %w", env, err)
			}
		}
	}

	// 环境变量覆盖
	l.bindEnvs()

	var cfg Config
	if err := l.viper.Unmarshal(&cfg, viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc())); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 设置环境变量
	if cfg.Env == "" {
		cfg.Env = EnvDevelopment
		if env != "" {
			cfg.Env = Environment(env)
		}
	}

	return &cfg, nil
}

func (l *FileLoader) Watch(stopChan <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					l.viper.ReadInConfig()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("config watch error: %v\n", err)
			case <-stopChan:
				watcher.Close()
				return
			}
		}
	}()

	configDir := filepath.Dir(l.configPath)
	return watcher.Add(configDir)
}

func (l *FileLoader) bindEnvs() {
	cfg := Config{}
	t := reflect.TypeOf(cfg)

	l.bindStructEnvs("", t, l.viper)
}

func (l *FileLoader) bindStructEnvs(prefix string, t reflect.Type, v *viper.Viper) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		envKey := field.Tag.Get("env")

		if envKey == "" {
			envKey = strings.ToUpper(field.Name)
		}

		if prefix != "" {
			envKey = fmt.Sprintf("%s_%s", prefix, envKey)
		}

		if field.Type.Kind() == reflect.Struct {
			l.bindStructEnvs(envKey, field.Type, v)
		} else {
			v.BindEnv(strings.ToLower(field.Name), envKey)
		}
	}
}
