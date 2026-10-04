package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath = "./configs/config.yaml"
)

// envPlaceholderPattern 匹配 ${VAR} 与 ${VAR:-default} 两种占位符。
// ${VAR} 缺失即视为配置错误（fail fast）；带 :- 的占位符在缺失时回落到默认值。
var envPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

var (
	cfg     *Config
	cfgErr  error
	cfgOnce sync.Once
)

// Load 初始化并返回全局配置
// Load reads configuration exactly once and caches the result.
func Load() (*Config, error) {
	cfgOnce.Do(func() {
		path := os.Getenv("CONFIG_PATH")
		if path == "" {
			path = defaultConfigPath
		}

		var content []byte
		// #nosec G703 -- CONFIG_PATH is deployment-controlled and deliberately supports absolute paths.
		content, cfgErr = os.ReadFile(filepath.Clean(path))
		if cfgErr != nil {
			cfgErr = fmt.Errorf("config: unable to read file %s: %w", path, cfgErr)
			return
		}

		// 在反序列化之前展开 ${VAR} 占位符，避免字面量 "${MYSQL_PASSWORD}"
		// 被当成真实密码连向数据库。缺失的必需变量在此终止启动（fail fast），
		// 而不是留到连接阶段报一条难以归因的认证错误。
		var expanded []byte
		expanded, cfgErr = expandEnv(content, os.LookupEnv)
		if cfgErr != nil {
			return
		}

		var parsed Config
		if err := yaml.Unmarshal(expanded, &parsed); err != nil {
			cfgErr = fmt.Errorf("config: unable to parse yaml: %w", err)
			return
		}

		if err := parsed.postProcess(); err != nil {
			cfgErr = err
			return
		}

		cfg = &parsed
	})

	return cfg, cfgErr
}

// expandEnv 把配置文本里的 ${VAR} / ${VAR:-default} 占位符替换为环境变量值。
//
// ${VAR} 是必需项：环境变量缺失时返回错误，避免把 "${MYSQL_PASSWORD}" 这样的
// 字面量当成真实密码连向数据库。${VAR:-default} 是可选项：缺失时回落默认值，
// 覆盖无密码 Redis、未启用邮件这类合法的空配置场景。
//
// lookup 做成参数一是便于测试，二是为了区分"未设置"与"显式设置为空字符串"：
// 后者会如实生效，不会被误判成缺失。
func expandEnv(content []byte, lookup func(string) (string, bool)) ([]byte, error) {
	var missing []string
	seen := make(map[string]struct{})

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		// 整行注释原样保留：注释是写给人看的，允许其中出现占位符形式的示例文本，
		// 否则每次在注释里举例都会让服务启动失败。
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		lines[i] = string(envPlaceholderPattern.ReplaceAllFunc([]byte(line), func(match []byte) []byte {
			sub := envPlaceholderPattern.FindSubmatch(match)
			name := string(sub[1])

			if value, ok := lookup(name); ok {
				return []byte(escapeYAMLDoubleQuoted(value))
			}

			// 带默认值 ${VAR:-x} 的形式：变量缺失即使用默认值。
			if len(sub) > 2 && sub[2] != nil {
				return sub[2]
			}

			if _, dup := seen[name]; !dup {
				seen[name] = struct{}{}
				missing = append(missing, name)
			}
			return match
		}))
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("config: unresolved required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// escapeYAMLDoubleQuoted 把值转义为可安全嵌入 YAML 双引号标量的形式。
// config.yaml 中的占位符都写在双引号内部；若不转义，环境变量值（例如包含引号
// 或换行的凭据）可以提前闭合引号并注入任意 YAML 键，进而改写无关的配置项。
func escapeYAMLDoubleQuoted(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\x%02x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (c *Config) postProcess() error {
	c.applyBaseDefaults()
	c.applyTaskSchedulerDefaults()
	c.applyInfrastructureDefaults()
	if err := c.applyTaskSchedulerOverrides(); err != nil {
		return err
	}
	if err := c.applyRealtimeDefaults(); err != nil {
		return err
	}

	if c.JWT.Secret == "" {
		return errors.New("config: jwt.secret must not be empty")
	}

	c.applyConnectionGatewayDefaults()
	c.applyEventsDefaults()
	if err := c.applyPrivacyDefaults(); err != nil {
		return err
	}

	return nil
}

func parseBoolEnv(key string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, false, nil
	}

	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, true, nil
	case "0", "false", "no", "off":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("config: invalid %s value: %s", key, raw)
	}
}

func parseIntEnv(key string) (int, bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, fmt.Errorf("config: invalid %s value: %w", key, err)
	}
	return value, true, nil
}
