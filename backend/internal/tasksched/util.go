package tasksched

import "github.com/google/uuid"

// randomSuffix 生成一个短随机后缀，用于默认 workerID。
func randomSuffix() string {
	return uuid.NewString()[:8]
}
