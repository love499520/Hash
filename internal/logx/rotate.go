package logx

import (
	"time"
)

// TodayLogPath
// 返回当天日志文件路径：logs/YYYY-MM-DD.log
func TodayLogPath() string {
	return "logs/" + time.Now().Format("2006-01-02") + ".log"
}
