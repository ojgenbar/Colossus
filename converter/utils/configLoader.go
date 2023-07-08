package utils

import (
	"github.com/gookit/config/v2"
	"github.com/gookit/config/v2/yamlv3"
)

type Config struct {
	S3      S3      `mapstructure:"s3"`
	Servers Servers `mapstructure:"servers"`
	Kafka   Kafka   `mapstructure:"kafka"`
}

type S3 struct {
	Auth    S3Auth    `mapstructure:"auth"`
	Buckets S3Buckets `mapstructure:"buckets"`
}

type S3Buckets struct {
	Raw       S3Bucket `mapstructure:"raw"`
	Processed S3Bucket `mapstructure:"processed"`
}

type S3Auth struct {
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"accessKeyID"`
	SecretAccessKey string `mapstructure:"secretAccessKey"`
}

type S3Bucket struct {
	Name     string `mapstructure:"bucketName"`
	Location string `mapstructure:"location"`
}

type Servers struct {
	System Server `mapstructure:"system"`
}

type Server struct {
	Addr string `mapstructure:"addr"`
}

type Kafka struct {
	BootstrapServers string `mapstructure:"bootstrap_servers"`
	ClientId         string `mapstructure:"client_id"`
	Topic            string `mapstructure:"topic"`
	GroupId          string `mapstructure:"group_id"`
	SessionTimeoutMs int    `mapstructure:"session_timeout_ms"`
	AutoOffsetReset  string `mapstructure:"auto_offset_reset"`
}

func LoadConfig() (*Config, error) {
	var cfg Config

	config.WithOptions(config.ParseEnv)

	config.AddDriver(yamlv3.Driver)

	err := config.LoadFiles("configs/main.yml")
	if err != nil {
		return nil, err
	}

	err = config.BindStruct("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
