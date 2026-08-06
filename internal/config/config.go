package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr         string        `yaml:"http_addr"`
	MySQLDSN         string        `yaml:"mysql_dsn"`
	RedisAddr        string        `yaml:"redis_addr"`
	RedisPass        string        `yaml:"redis_pass"`
	JWTSecret        string        `yaml:"jwt_secret"`
	JWTTTL           time.Duration `yaml:"jwt_ttl"`
	MinioEndpoint    string        `yaml:"minio_endpoint"`
	MinioAccessKey   string        `yaml:"minio_access_key"`
	MinioSecretKey   string        `yaml:"minio_secret_key"`
	MinioBucket      string        `yaml:"minio_bucket"`
	MinioUseSSL      bool          `yaml:"minio_use_ssl"`
	LiveKitAPIKey    string        `yaml:"livekit_api_key"`
	LiveKitAPISecret string        `yaml:"livekit_api_secret"`
	LiveKitPublicURL string        `yaml:"livekit_public_url"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		HTTPAddr:         ":8080",
		MySQLDSN:         "root:root@tcp(127.0.0.1:3306)/chatroom?parseTime=true&charset=utf8mb4",
		RedisAddr:        "127.0.0.1:6379",
		JWTSecret:        "dev-secret-change-me",
		JWTTTL:           24 * time.Hour,
		MinioEndpoint:    "127.0.0.1:9000",
		MinioAccessKey:   "minioadmin",
		MinioSecretKey:   "minioadmin",
		MinioBucket:      "chatroom-media",
		LiveKitAPIKey:    "devkey",
		LiveKitAPISecret: "devsecretkeydevsecretkeydevsecretkeydevsecretkey",
		LiveKitPublicURL: "ws://localhost:7880",
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
	if v := os.Getenv("CHATROOM_MINIO_ENDPOINT"); v != "" {
		cfg.MinioEndpoint = v
	}
	if v := os.Getenv("CHATROOM_MINIO_ACCESS_KEY"); v != "" {
		cfg.MinioAccessKey = v
	}
	if v := os.Getenv("CHATROOM_MINIO_SECRET_KEY"); v != "" {
		cfg.MinioSecretKey = v
	}
	if v := os.Getenv("CHATROOM_MINIO_BUCKET"); v != "" {
		cfg.MinioBucket = v
	}
	if v := os.Getenv("CHATROOM_MINIO_USE_SSL"); v != "" {
		cfg.MinioUseSSL = v == "true" || v == "1"
	}
	if v := os.Getenv("CHATROOM_LIVEKIT_API_KEY"); v != "" {
		cfg.LiveKitAPIKey = v
	}
	if v := os.Getenv("CHATROOM_LIVEKIT_API_SECRET"); v != "" {
		cfg.LiveKitAPISecret = v
	}
	if v := os.Getenv("CHATROOM_LIVEKIT_PUBLIC_URL"); v != "" {
		cfg.LiveKitPublicURL = v
	}
}
