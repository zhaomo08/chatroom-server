package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr  string        `yaml:"http_addr"`
	MySQLDSN  string        `yaml:"mysql_dsn"`
	RedisAddr string        `yaml:"redis_addr"`
	RedisPass string        `yaml:"redis_pass"`
	JWTSecret string        `yaml:"jwt_secret"`
	JWTTTL    time.Duration `yaml:"jwt_ttl"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		HTTPAddr:  ":8080",
		MySQLDSN:  "root:root@tcp(127.0.0.1:3306)/chatroom?parseTime=true&charset=utf8mb4",
		RedisAddr: "127.0.0.1:6379",
		JWTSecret: "dev-secret-change-me",
		JWTTTL:    24 * time.Hour,
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
	}
	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CHATROOM_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("CHATROOM_MYSQL_DSN"); v != "" {
		cfg.MySQLDSN = v
	}
	if v := os.Getenv("CHATROOM_REDIS_ADDR"); v != "" {
		cfg.RedisAddr = v
	}
	if v := os.Getenv("CHATROOM_REDIS_PASS"); v != "" {
		cfg.RedisPass = v
	}
	if v := os.Getenv("CHATROOM_JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("CHATROOM_JWT_TTL_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			cfg.JWTTTL = time.Duration(secs) * time.Second
		}
	}
}
