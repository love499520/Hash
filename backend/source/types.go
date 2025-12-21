package source

// Config：一个 HTTP 数据源配置（900 系列）
type Config struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// 请求
	Method  string            `json:"method"`  // GET / POST
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"` // POST body（JSON 字符串）

	// 解析（dot path）
	HeightPath string `json:"heightPath"` // e.g. block_header.raw_data.number
	HashPath   string `json:"hashPath"`   // e.g. blockID
	TimePath   string `json:"timePath"`   // e.g. block_header.raw_data.timestamp

	// 时间单位：ms 或 s（Tron 常见 ms）
	TimeUnit string `json:"timeUnit"` // "ms" or "s"

	// 906：双阈值（滑块）
	BaseRPS int `json:"baseRps"` // 基础轮询频率：每秒请求次数
	MaxRPS  int `json:"maxRps"`  // 上限保护：最大每秒请求次数

	// 超时
	TimeoutMS int `json:"timeoutMs"`
}
