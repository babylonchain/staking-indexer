package config

import (
	"fmt"
	"time"
)

const (
	defaultQueueUser              = "user"
	defaultQueuePassword          = "password"
	defaultQueueUrl               = "localhost:5672"
	defaultQueueProcessingTimeout = 5 * time.Second
)

type QueueConfig struct {
	User              string        `long:"user" description:"the user name of the queue"`
	Password          string        `long:"password" description:"the password of the queue"`
	Url               string        `long:"url" description:"the url of the queue"`
	ProcessingTimeout time.Duration `long:"processingtimeout" description:"the process timeout of the queue"`
}

func (cfg *QueueConfig) Validate() error {
	if cfg.User == "" {
		return fmt.Errorf("missing queue user")
	}

	if cfg.Password == "" {
		return fmt.Errorf("missing queue password")
	}

	if cfg.Url == "" {
		return fmt.Errorf("missing queue url")
	}

	if cfg.ProcessingTimeout <= 0 {
		return fmt.Errorf("invalid queue processing timeout")
	}

	return nil
}

func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		User:              defaultQueueUser,
		Password:          defaultQueuePassword,
		Url:               defaultQueueUrl,
		ProcessingTimeout: defaultQueueProcessingTimeout,
	}
}
