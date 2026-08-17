package signaling

import (
	"context"
	"strings"
)

// StartPresenceFeed 将 presence:feed:* 的状态变更投递到本节点已连接的设备。
// 复用既有 Redis pub/sub 跨实例桥接与按 email 寻址的客户端表，实现实时广播。
func (h *Hub) StartPresenceFeed(ctx context.Context) {
	if h.redis == nil {
		return
	}
	const prefix = "presence:feed:"
	sub := h.redis.PSubscribe(ctx, prefix+"*")
	go func() {
		defer sub.Close()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				email := strings.TrimPrefix(msg.Channel, prefix)
				if email == "" || email == msg.Channel {
					continue
				}
				h.dispatchLocal(email, []byte(msg.Payload))
			}
		}
	}()
}
