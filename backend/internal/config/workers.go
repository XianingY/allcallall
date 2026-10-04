package config

import "os"

// TaskSchedulerConfig 周期性（weekly）任务调度器配置
// TaskSchedulerConfig controls the weekly task scheduler worker.
type TaskSchedulerConfig struct {
	Enabled            bool   `yaml:"enabled" env:"TASK_SCHEDULER_ENABLED"`
	IntervalSec        int    `yaml:"interval_seconds" env:"TASK_SCHEDULER_INTERVAL_SEC"`
	Timezone           string `yaml:"timezone" env:"TASK_SCHEDULER_TIMEZONE"`
	WorkerID           string `yaml:"worker_id" env:"TASK_SCHEDULER_WORKER_ID"`
	MaxConcurrent      int    `yaml:"max_concurrent" env:"TASK_SCHEDULER_MAX_CONCURRENT"`
	LeaseSec           int    `yaml:"lease_seconds" env:"TASK_SCHEDULER_LEASE_SEC"`
	DefaultMaxFailures int    `yaml:"default_max_failures" env:"TASK_SCHEDULER_DEFAULT_MAX_FAILURES"`
}

// EventsConfig 事件总线生产化（Kafka 桥接）配置
// EventsConfig controls fan-out of domain events to Kafka when enabled.
type EventsConfig struct {
	KafkaEnabled      bool   `yaml:"kafka_enabled" env:"EVENTS_KAFKA_ENABLED"`
	TopicPrefix       string `yaml:"topic_prefix" env:"EVENTS_KAFKA_TOPIC_PREFIX"`
	BridgeChat        bool   `yaml:"bridge_chat" env:"EVENTS_BRIDGE_CHAT"`
	BridgeWeeklyTasks bool   `yaml:"bridge_weekly_tasks" env:"EVENTS_BRIDGE_WEEKLY_TASKS"`
}

func (c *Config) applyTaskSchedulerDefaults() {
	// 周期性任务调度器默认配置
	// Weekly task scheduler defaults
	if c.TaskScheduler.IntervalSec <= 0 {
		c.TaskScheduler.IntervalSec = 60
	}
	if c.TaskScheduler.Timezone == "" {
		c.TaskScheduler.Timezone = "UTC"
	}
	if c.TaskScheduler.MaxConcurrent <= 0 {
		c.TaskScheduler.MaxConcurrent = 8
	}
	if c.TaskScheduler.LeaseSec <= 0 {
		c.TaskScheduler.LeaseSec = 120
	}
	if c.TaskScheduler.DefaultMaxFailures <= 0 {
		c.TaskScheduler.DefaultMaxFailures = 5
	}
	if c.TaskScheduler.WorkerID == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			c.TaskScheduler.WorkerID = "scheduler-" + host
		} else {
			c.TaskScheduler.WorkerID = "scheduler-unknown"
		}
	}
}

func (c *Config) applyTaskSchedulerOverrides() error {
	// 支持环境变量覆盖周期性任务调度器配置
	// Support environment variables override weekly task scheduler config
	if enabled, ok, err := parseBoolEnv("TASK_SCHEDULER_ENABLED"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.Enabled = enabled
	}
	if v := os.Getenv("TASK_SCHEDULER_WORKER_ID"); v != "" {
		c.TaskScheduler.WorkerID = v
	}
	if v := os.Getenv("TASK_SCHEDULER_TIMEZONE"); v != "" {
		c.TaskScheduler.Timezone = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_INTERVAL_SEC"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.IntervalSec = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_MAX_CONCURRENT"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.MaxConcurrent = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_LEASE_SEC"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.LeaseSec = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_DEFAULT_MAX_FAILURES"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.DefaultMaxFailures = v
	}
	return nil
}

func (c *Config) applyEventsDefaults() {
	if c.Events.TopicPrefix == "" {
		c.Events.TopicPrefix = "allcallall"
	}

	if enabled, ok, err := parseBoolEnv("EVENTS_KAFKA_ENABLED"); err != nil {
		return
	} else if ok {
		c.Events.KafkaEnabled = enabled
	}
	if v := os.Getenv("EVENTS_KAFKA_TOPIC_PREFIX"); v != "" {
		c.Events.TopicPrefix = v
	}
	if enabled, ok, err := parseBoolEnv("EVENTS_BRIDGE_CHAT"); err != nil {
		return
	} else if ok {
		c.Events.BridgeChat = enabled
	}
	if enabled, ok, err := parseBoolEnv("EVENTS_BRIDGE_WEEKLY_TASKS"); err != nil {
		return
	} else if ok {
		c.Events.BridgeWeeklyTasks = enabled
	}
}
