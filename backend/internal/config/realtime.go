package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// WebRTCConfig WebRTC 相关配置
// WebRTCConfig contains ICE server list.
type WebRTCConfig struct {
	ICEServers []ICEServer `yaml:"ice_servers" json:"ice_servers"`
}

// ICEServer 单个 ICE 服务配置
// ICEServer represents a single ICE server entry.
type ICEServer struct {
	URLs       []string `yaml:"urls" json:"urls"`
	Username   string   `yaml:"username" json:"username"`
	Credential string   `yaml:"credential" json:"credential"`
}

// ConnectionGatewayConfig 连接层负载均衡网关配置
// ConnectionGatewayConfig controls self node registration and consistent-hash routing.
type ConnectionGatewayConfig struct {
	Enabled       bool   `yaml:"enabled" env:"CONNECTION_GATEWAY_ENABLED"`
	SelfID        string `yaml:"self_id" env:"CONNECTION_GATEWAY_SELF_ID"`
	AdvertiseAddr string `yaml:"advertise_addr" env:"CONNECTION_GATEWAY_ADVERTISE_ADDR"`
	HeartbeatSec  int    `yaml:"heartbeat_seconds" env:"CONNECTION_GATEWAY_HEARTBEAT_SEC"`
	NodeTTLSec    int    `yaml:"node_ttl_seconds" env:"CONNECTION_GATEWAY_NODE_TTL_SEC"`
	HashReplicas  int    `yaml:"hash_replicas" env:"CONNECTION_GATEWAY_HASH_REPLICAS"`
}

// TranslationConfig 实时翻译配置
// TranslationConfig controls realtime translation runtime behavior.
type TranslationConfig struct {
	Enabled            bool          `yaml:"enabled"`
	Provider           string        `yaml:"provider"`
	ChunkMS            int           `yaml:"chunk_ms"`
	PartialDebounceMS  int           `yaml:"partial_debounce_ms"`
	MaxSessionsPerUser int           `yaml:"max_sessions_per_user"`
	VolcAST            VolcASTConfig `yaml:"volc_ast"`
}

// VolcASTConfig 火山 AST 配置
// VolcASTConfig stores Volcengine AST provider options.
type VolcASTConfig struct {
	WSURL      string `yaml:"ws_url"`
	AppKey     string `yaml:"app_key"`
	AccessKey  string `yaml:"access_key"`
	ResourceID string `yaml:"resource_id"`
	AppID      string `yaml:"app_id"`
}

func (c *Config) applyRealtimeDefaults() error {
	// 支持环境变量覆盖 ICE/TURN 配置，格式为 JSON 数组：
	// [{"urls":["stun:stun.l.google.com:19302"]},{"urls":["turn:1.2.3.4:3478"],"username":"user","credential":"pass"}]
	if iceServersJSON := os.Getenv("WEBRTC_ICE_SERVERS_JSON"); iceServersJSON != "" {
		// Docker Compose / env files sometimes preserve surrounding quotes.
		// Example (broken JSON): '[{"urls":["stun:..."]}]'
		iceServersJSON = strings.Trim(iceServersJSON, "\"'")

		var servers []ICEServer
		if err := json.Unmarshal([]byte(iceServersJSON), &servers); err != nil {
			// Backward/compat: some configs use an object wrapper: {"ice_servers": [...]}
			var wrapper struct {
				ICEServers []ICEServer `json:"ice_servers"`
			}
			if err2 := json.Unmarshal([]byte(iceServersJSON), &wrapper); err2 != nil {
				return fmt.Errorf("config: invalid WEBRTC_ICE_SERVERS_JSON: %w", err)
			}
			servers = wrapper.ICEServers
		}
		if len(servers) > 0 {
			c.WebRTC.ICEServers = servers
		}
	}

	if c.Translation.Provider == "" {
		c.Translation.Provider = "volc_ast"
	}
	if c.Translation.ChunkMS <= 0 {
		c.Translation.ChunkMS = 400
	}
	if c.Translation.PartialDebounceMS <= 0 {
		c.Translation.PartialDebounceMS = 600
	}
	if c.Translation.MaxSessionsPerUser <= 0 {
		c.Translation.MaxSessionsPerUser = 2
	}
	if c.Translation.VolcAST.WSURL == "" {
		c.Translation.VolcAST.WSURL = "wss://openspeech.bytedance.com/api/v4/ast/v2/translate"
	}
	if c.Translation.VolcAST.ResourceID == "" {
		c.Translation.VolcAST.ResourceID = "volc.service_type.10053"
	}

	// 支持环境变量覆盖翻译配置
	// Support environment variables override translation config
	if enabled, ok, err := parseBoolEnv("TRANSLATION_ENABLED"); err != nil {
		return err
	} else if ok {
		c.Translation.Enabled = enabled
	}
	if provider := os.Getenv("TRANSLATION_PROVIDER"); provider != "" {
		c.Translation.Provider = provider
	}
	if chunkMS, ok, err := parseIntEnv("TRANSLATION_CHUNK_MS"); err != nil {
		return err
	} else if ok {
		c.Translation.ChunkMS = chunkMS
	}
	if partialDebounceMS, ok, err := parseIntEnv("TRANSLATION_PARTIAL_DEBOUNCE_MS"); err != nil {
		return err
	} else if ok {
		c.Translation.PartialDebounceMS = partialDebounceMS
	}
	if maxSessions, ok, err := parseIntEnv("TRANSLATION_MAX_SESSIONS_PER_USER"); err != nil {
		return err
	} else if ok {
		c.Translation.MaxSessionsPerUser = maxSessions
	}

	if volcWSURL := os.Getenv("VOLC_AST_WS_URL"); volcWSURL != "" {
		c.Translation.VolcAST.WSURL = volcWSURL
	}
	if volcAppKey := os.Getenv("VOLC_AST_APP_KEY"); volcAppKey != "" {
		c.Translation.VolcAST.AppKey = volcAppKey
	}
	if volcAccessKey := os.Getenv("VOLC_AST_ACCESS_KEY"); volcAccessKey != "" {
		c.Translation.VolcAST.AccessKey = volcAccessKey
	}
	if volcResourceID := os.Getenv("VOLC_AST_RESOURCE_ID"); volcResourceID != "" {
		c.Translation.VolcAST.ResourceID = volcResourceID
	}
	if volcAppID := os.Getenv("VOLC_AST_APP_ID"); volcAppID != "" {
		c.Translation.VolcAST.AppID = volcAppID
	}
	return nil
}

func (c *Config) applyConnectionGatewayDefaults() {
	if c.ConnectionGateway.HeartbeatSec <= 0 {
		c.ConnectionGateway.HeartbeatSec = 10
	}
	if c.ConnectionGateway.NodeTTLSec <= 0 {
		c.ConnectionGateway.NodeTTLSec = 30
	}
	if c.ConnectionGateway.HashReplicas <= 0 {
		c.ConnectionGateway.HashReplicas = 100
	}
	if c.ConnectionGateway.SelfID == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			c.ConnectionGateway.SelfID = "gateway-" + host
		} else {
			c.ConnectionGateway.SelfID = "gateway-unknown"
		}
	}
	if c.ConnectionGateway.AdvertiseAddr == "" {
		c.ConnectionGateway.AdvertiseAddr = fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
	}

	if enabled, ok, err := parseBoolEnv("CONNECTION_GATEWAY_ENABLED"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.Enabled = enabled
	}
	if v := os.Getenv("CONNECTION_GATEWAY_SELF_ID"); v != "" {
		c.ConnectionGateway.SelfID = v
	}
	if v := os.Getenv("CONNECTION_GATEWAY_ADVERTISE_ADDR"); v != "" {
		c.ConnectionGateway.AdvertiseAddr = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_HEARTBEAT_SEC"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.HeartbeatSec = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_NODE_TTL_SEC"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.NodeTTLSec = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_HASH_REPLICAS"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.HashReplicas = v
	}
}
