package utils

import (
	"bytes"
	"fmt"
	"github.com/gookit/config/v2"
	"github.com/gookit/config/v2/yamlv3"
)

type Config struct {
	S3 S3 `mapstructure:"s3"`
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

	fmt.Printf("config data: \n %#v\n", config.Data())
	//fmt.Printf("config data: \n %v\n", MyConfig)
	//fmt.Println(MyConfig.S3.auth.accessKeyID)
	fmt.Println(cfg.S3.Auth.AccessKeyID)
	fmt.Println(cfg.S3.Auth.Endpoint)
	//fmt.Println(config.String("s3.auth.accessKeyID"))
	fmt.Println()

	buf := new(bytes.Buffer)
	_, err = config.DumpTo(buf, config.JSON)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", buf.String())
	return &cfg, nil
}
