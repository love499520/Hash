package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"

	"tron-signal/backend/app"
	"tron-signal/backend/auth"
	"tron-signal/backend/config"
)

func loginHandler(core *app.Core, store *config.Store, sessions *auth.MemoryStore) http.HandlerFunc {
	_ = core
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)

		cfg := store.Get()

		// 首次未设置账号密码：强制走“首次设置”
		firstSetup := strings.TrimSpace(cfg.AdminUser) == "" || strings.TrimSpace(cfg.AdminPassHash) == ""

		switch r.Method {
		case http.MethodGet:
			renderLogin(w, loginPageData{
				FirstSetup: firstSetup,
				Error:      "",
			})
			return

		case http.MethodPost:
			_ = r.ParseForm()
			user := strings.TrimSpace(r.FormValue("user"))
			pass := strings.TrimSpace(r.FormValue("pass"))

			if user == "" || pass == "" {
				renderLogin(w, loginPageData{FirstSetup: firstSetup, Error: "账号或密码不能为空"})
				return
			}

			if firstSetup {
				// 首次设置：直接落盘
				newHash := config.HashPassword(pass)
				store.Update(func(c *config.Config) {
					c.AdminUser = user
					c.AdminPassHash = newHash
				})
				sessions.Login(w)
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}

			// 正常登录
			if user != cfg.AdminUser || !config.VerifyPassword(pass, cfg.AdminPassHash) {
				renderLogin(w, loginPageData{FirstSetup: false, Error: "账号或密码错误"})
				return
			}

			sessions.Login(w)
			http.Redirect(w, r, "/", http.StatusFound)
			return

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
}

func logoutHandler(store *config.Store, sessions *auth.MemoryStore) http.HandlerFunc {
	_ = store
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		sessions.Logout(w)
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// 管理：修改登录密码（需要已登录）
func adminPasswordHandler(core *app.Core, store *config.Store) http.HandlerFunc {
	_ = core
	return func(w http.ResponseWriter, r *http.Request) {
		NoCache(w)
		cfg := store.Get()

		if r.Method == http.MethodGet {
			renderPassword(w, passPageData{Error: "", Ok: ""})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		_ = r.ParseForm()
		oldPass := strings.TrimSpace(r.FormValue("old"))
		newPass := strings.TrimSpace(r.FormValue("new"))

		if oldPass == "" || newPass == "" {
			renderPassword(w, passPageData{Error: "旧密码/新密码不能为空", Ok: ""})
			return
		}

		if strings.TrimSpace(cfg.AdminUser) == "" || strings.TrimSpace(cfg.AdminPassHash) == "" {
			renderPassword(w, passPageData{Error: "未初始化账号密码，请先到登录页首次设置", Ok: ""})
			return
		}

		if !config.VerifyPassword(oldPass, cfg.AdminPassHash) {
			renderPassword(w, passPageData{Error: "旧密码错误", Ok: ""})
			return
		}

		store.Update(func(c *config.Config) {
			c.AdminPassHash = config.HashPassword(newPass)
		})

		renderPassword(w, passPageData{Error: "", Ok: "已更新密码"})
	}
}

// ====== HTML 渲染（移动端友好，最小依赖）======

type loginPageData struct {
	FirstSetup bool
	Error      string
}

type passPageData struct {
	Error string
	Ok    string
}

func renderLogin(w http.ResponseWriter, d loginPageData) {
	tpl := `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>Tron Signal 登录</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial; margin:0; background:#0b0f14; color:#e8eef6;}
.wrap{max-width:420px; margin:0 auto; padding:24px 16px;}
.card{background:#111826; border:1px solid #1f2a3a; border-radius:14px; padding:18px;}
h1{font-size:18px; margin:0 0 12px;}
.p{font-size:13px; color:#9fb0c6; margin:0 0 14px; line-height:1.4;}
label{display:block; font-size:13px; margin:10px 0 6px; color:#cdd9e5;}
input{width:100%; box-sizing:border-box; padding:12px 12px; border-radius:10px; border:1px solid #223047; background:#0b1220; color:#e8eef6; font-size:15px;}
.btn{margin-top:14px; width:100%; padding:12px; border:0; border-radius:10px; background:#2b6df3; color:white; font-size:15px; font-weight:600;}
.err{margin-top:12px; padding:10px 12px; border-radius:10px; background:#2a0f14; border:1px solid #5a1c24; color:#ffb4b4; font-size:13px;}
</style>
</head>
<body>
<div class="wrap">
  <div class="card">
    <h1>{{if .FirstSetup}}首次设置账号密码{{else}}登录{{end}}</h1>
    <p class="p">{{if .FirstSetup}}首次进入必须完成设置，设置成功后将持久化保存。{{else}}请输入账号密码进入管理界面。{{end}}</p>

    <form method="post" action="/login" autocomplete="off">
      <label>账号</label>
      <input name="user" placeholder="请输入账号" />

      <label>密码</label>
      <input type="password" name="pass" placeholder="请输入密码" />

      <button class="btn" type="submit">{{if .FirstSetup}}保存并进入{{else}}登录{{end}}</button>
    </form>

    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
  </div>
</div>
</body></html>`
	_ = template.Must(template.New("login").Parse(tpl)).Execute(w, d)
}

func renderPassword(w http.ResponseWriter, d passPageData) {
	tpl := `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>修改密码</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial; margin:0; background:#0b0f14; color:#e8eef6;}
.wrap{max-width:520px; margin:0 auto; padding:24px 16px;}
.card{background:#111826; border:1px solid #1f2a3a; border-radius:14px; padding:18px;}
h1{font-size:18px; margin:0 0 12px;}
label{display:block; font-size:13px; margin:10px 0 6px; color:#cdd9e5;}
input{width:100%; box-sizing:border-box; padding:12px 12px; border-radius:10px; border:1px solid #223047; background:#0b1220; color:#e8eef6; font-size:15px;}
.btn{margin-top:14px; width:100%; padding:12px; border:0; border-radius:10px; background:#2b6df3; color:white; font-size:15px; font-weight:600;}
.msg{margin-top:12px; padding:10px 12px; border-radius:10px; font-size:13px;}
.err{background:#2a0f14; border:1px solid #5a1c24; color:#ffb4b4;}
.ok{background:#0f2a18; border:1px solid #1f5a33; color:#b8ffd2;}
a{color:#9fb0c6; text-decoration:none; font-size:13px;}
</style>
</head>
<body>
<div class="wrap">
  <div class="card">
    <h1>修改登录密码</h1>
    <form method="post" action="/admin/password" autocomplete="off">
      <label>旧密码</label>
      <input type="password" name="old" placeholder="请输入旧密码" />
      <label>新密码</label>
      <input type="password" name="new" placeholder="请输入新密码" />
      <button class="btn" type="submit">保存</button>
    </form>

    {{if .Error}}<div class="msg err">{{.Error}}</div>{{end}}
    {{if .Ok}}<div class="msg ok">{{.Ok}}</div>{{end}}

    <div style="margin-top:14px;"><a href="/">返回首页</a></div>
  </div>
</div>
</body></html>`
	_ = template.Must(template.New("pass").Parse(tpl)).Execute(w, d)
}

// 备用：生成随机串（未来可用于 CSRF / session token）
func randHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
