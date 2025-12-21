package app

import (
	"os"

	"tron-signal/internal/logx"
)

const runningLock = "data/running.lock"

// CheckAbnormalRestart
// 启动时检查是否存在 running.lock
// 若存在，说明上次未正常退出
func CheckAbnormalRestart() {
	if _, err := os.Stat(runningLock); err == nil {
		logx.Info("ABNORMAL_RESTART")
	}
	// 写入运行锁
	_ = os.WriteFile(runningLock, []byte("running"), 0644)
}

// MarkNormalExit
// 正常退出时清理 running.lock
func MarkNormalExit() {
	_ = os.Remove(runningLock)
}
