package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"tron-signal/backend/app"
	"tron-signal/backend/auth"
	"tron-signal/backend/block"
	"tron-signal/backend/http"
	"tron-signal/backend/judge"
	"tron-signal/backend/machine"
	"tron-signal/backend/source"
	"tron-signal/backend/ws"
	"tron-signal/backend/config"
)

func main() {
	// ====== 基础目录 ======
	_ = os.MkdirAll("data", 0755)
	_ = os.MkdirAll("logs", 0755)

	// ====== 日志（简单版，轮转模块后续继续补） ======
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// ====== 异常重启 lock（1102） ======
	lock := "data/running.lock"
	if _, err := os.Stat(lock); err == nil {
		log.Printf("ABNORMAL_RESTART\n")
	}
	_ = os.WriteFile(lock, []byte(time.Now().Format(time.RFC3339)), 0644)
	defer os.Remove(lock)

	log.Printf("SYSTEM_START\n")

	// ====== 配置加载（1104） ======
	cfg := config.MustLoad("data/config.json")

	// ====== 核心组件 ======
	ring := block.NewRingBuffer(50)

	j := judge.NewJudge(cfg.JudgeRule)

	mgr := machine.NewManager(cfg.Machines)

	hub := ws.NewHub()

	core := app.NewCore(ring, j, mgr, hub)

	// ====== 数据源 Dispatcher（三源：ankr-rest / ankr-rpc / trongrid） ======
	dispatcher := source.NewDispatcher()

	// 1) Ankr REST
	for _, s := range cfg.Sources {
		switch s.Type {
		case "ankr-rest":
			dispatcher.Add(source.NewAnkrRestFetcher(s))
		case "ankr-rpc":
			// method/params 来自 cfg.SourceExtras（后续模块会给）
			method := cfg.SourceExtras[s.ID].RPCMethod
			params := cfg.SourceExtras[s.ID].RPCParams
			dispatcher.Add(source.NewAnkrRpcFetcher(s, method, params))
		case "trongrid":
			dispatcher.Add(source.NewTronGridFetcher(s))
		}
	}

	// ====== 轮询 Runner（失败等待策略从 cfg 取） ======
	runner := app.NewRunner(core, dispatcher)
	runner.UpdatePolicy(cfg.Poll.AutoRestart, cfg.Poll.FailWaitMinutes)
	runner.UpdateBaseTick(cfg.Poll.BaseTickMS)

	go runner.Run()

	// ====== 鉴权（白名单/Token + 管理登录态） ======
	authStore := auth.NewAuthStore()

	// Router 依赖（注意：Router 会把所有入口套 RequireTokenOrWhitelist）
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Core:      core,
		Hub:       hub,
		AuthStore: authStore,
		Cfg:       cfg, // cfg 实现 auth.ConfigReader（后续 config 模块会实现）
		WebDir:    "web",
		DocsDir:   "api/docs",
		LogDir:    "logs",
	})

	// ====== 启动门禁（1105） ======
	// 规则：首次必须 UI 配置并持久化；配置完成后无人登录也能跑。
	// 这里的“能否进入轮询”由 cfg 内是否存在至少一个 enabled source + key 决定，
	// dispatcher 内部会对 disabled 源返回 disabled。
	// 如果你希望更严格（比如没源就暂停 runner），后面我在 config/dispatcher 衔接处补一个 gate。

	log.Printf("HTTP_LISTEN :8080\n")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Printf("SERVER_ERROR: %v\n", err)
	}
}
