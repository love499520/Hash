package block

// Block：标准化区块结构（跨数据源统一）
// 注意：Height/Hash 以 string 保留（避免不同源返回类型差异导致丢精度）
type Block struct {
	Height string `json:"height"`
	Hash   string `json:"hash"`
	// TimeUnix：unix 秒（后端统一秒级，UI 强制北京时间格式化）
	TimeUnix int64 `json:"timeUnix"`
	SourceID string `json:"sourceId,omitempty"`
}
