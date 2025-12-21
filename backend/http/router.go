package httpapi

import (
	"net/http"
	"time"

	"tron-signal/backend/auth"
	"tron-signal/backend/app"
	"tron-signal/backend/sse"
	"tron-signal/backend/ws"
)

// RouterDeps：组装依赖
type RouterDeps struct {
	Core      *app.Core
	Hub       *ws.Hub

	AuthStore *auth.AuthStore
	Cfg       auth.ConfigReader

	// 静态资源
	WebDir  string // web/
	DocsDir string // api/docs/
	LogDir  string // logs/
}

// NewRouter：返回 http.Handler（可直接 ListenAndServe）
func NewRouter(d RouterDeps) http.Handler {
	mux := http.NewServeMux()

	pub := &PublicHandlers{Core: d.Core}
	admin := &AdminHandlers{
		AuthStore: d.AuthStore,
		Cfg:       d.Cfg,
		LogDir:    d.LogDir,
	}

	// ========== Public API ==========
	mux.HandleFunc("/api/status", pub.Status)
	mux.HandleFunc("/api/blocks", pub.Blocks)

	// SSE：UI 状态刷新（稳定）
	mux.Handle("/sse/status", sse.StatusHandler(d.Core, 800*time.Millisecond))
	mux.Handle("/sse/blocks", sse.BlocksHandler(d.Core, 800*time.Millisecond))

	// WS：仅信号广播
	mux.HandleFunc("/ws/signal", d.Hub.HandleWS)

	// UI 静态（/）
	if d.WebDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(d.WebDir)))
	}

	// ========== Admin (login required) ==========
	// 登录接口本身不需要 admin_session，但你也可以选择仅白名单/Token 放行
	mux.HandleFunc("/api/admin/login", admin.Login)
	mux.HandleFunc("/api/admin/logout", admin.Logout)

	// admin_session 保护：日志/Docs
	adminOnly := auth.RequireAdminSession(d.AuthStore)

	mux.Handle("/api/admin/logs", adminOnly(http.HandlerFunc(admin.Logs)))

	// /docs 与 /api/docs/api.md：仅允许管理登录态
	if d.DocsDir != "" {
		mux.Handle("/docs/", adminOnly(http.StripPrefix("/docs/", http.FileServer(http.Dir(d.DocsDir)))))
		// 单文件访问（兼容你清单）
		mux.Handle("/api/docs/api.md", adminOnly(http.FileServer(http.Dir(d.DocsDir))))
	}

	// ========== 外部统一门禁（内网白名单/外网Token） ==========
	// 注意：不区分 HTTP/WS——这里统一套住整个 mux。
	guard := auth.RequireTokenOrWhitelist(d.Cfg)
	return guard(mux)
}
