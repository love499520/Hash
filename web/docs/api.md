# Tron Signal API 文档

> 本文档仅允许管理登录态访问（/docs）

## 鉴权模型
系统只区分【内网白名单 / 外网访问】，不区分 HTTP / WS：
- 内网 IP 白名单：免 Token
- 外网访问：必须携带 Token
- Token 传递方式（任意一种）：
  - Header：Authorization: Bearer <TOKEN>
  - Header：X-Token: <TOKEN>
  - Query：?token=<TOKEN>

## 基础状态

### GET /api/status
返回系统运行状态与实时信息

返回示例：
```json
{
  "Listening": true,
  "LastHeight": 62345678,
  "LastHash": "abcd1***9f3e2",
  "LastTimeISO": "2025/01/01 12:00:01",
  "Reconnects": 0,
  "ConnectedKeys": 3,
  "JudgeRule": "幸运",
  "Machines": 2
}
字段说明：
Listening：是否正在轮询区块
LastHeight：最近区块高度
LastHash：最近区块哈希（UI 已做首尾截断）
LastTimeISO：北京时间（UTC+8，秒级）
Reconnects：轮询异常重试次数
ConnectedKeys：当前可用数据源数量
JudgeRule：当前 ON/OFF 判定规则
Machines：状态机数量
判定规则切换
POST /api/judge/switch
用于全局切换 ON / OFF 判定规则
请求体：
{
  "rule": "lucky | big | odd",
  "confirm": true
}
说明：
rule 可选值：
lucky（幸运）
big（大小）
odd（单双）
confirm=false 时仅返回提示，不执行切换
confirm=true 才会真正执行切换
行为约束：
切换判定规则会自动：
停止所有状态机
清空所有计数器
清空运行态
清空区块去重缓存
状态机配置
GET /api/machines
获取当前全部状态机配置与运行态
PUT /api/machines
整表保存状态机配置（推荐方式）
请求体示例：
{
  "machines": [
    {
      "id": "m1",
      "enabled": true,
      "trigger": { "state": "ON", "threshold": 3 },
      "hit": { "enabled": true, "expect": "OFF", "offset": 2 }
    }
  ]
}
说明：
支持多个状态机
HIT 规则仅在基础触发规则命中后执行一次
HIT 关闭时不做任何判断
offset 为 T+X 的 X，仅影响判断，不影响 UI 展示规则
数据源管理
GET /api/sources
获取所有 HTTP 数据源配置
POST /api/sources/upsert
新增或更新数据源
POST /api/sources/delete
删除指定数据源
数据源特性：
支持 Ankr REST / JSON-RPC、TronGrid HTTP
不区分主源/兜底源
采用“先到先用”策略
每个源具备两个阈值：
base：基础轮询频率
max：上限频率（保护性限流）
Token 管理（仅管理登录态）
GET /api/tokens
查看 Token 列表（不会明文显示历史 Token）
POST /api/tokens
生成新 Token
DELETE /api/tokens?token=xxx
删除指定 Token
白名单管理
GET /api/whitelist
获取 IP 白名单
PUT /api/whitelist
覆盖式保存白名单列表
日志
GET /api/logs
查询日志内容
参数：
q：关键字过滤
major=1：仅显示重大事故日志
重大事故包含：
ABNORMAL_RESTART
区块高度跳跃
区块丢失
数据源不可用
轮询连续失败
实时状态流（SSE）
GET /sse/status
用于 UI 实时状态更新
说明：
仅使用 SSE 推送状态
UI 区块卡片不自动跳回顶部
WebSocket 仅用于信号广播，不承载 UI 状态
WebSocket（广播）
GET /ws
仅用于向外广播信号数据
说明：
不参与任何鉴权逻辑判断
不参与状态计算
不保证重发、不保证确认
重要声明
Tron 链不支持区块 WebSocket 订阅
本系统彻底不使用 WS 获取区块
所有区块均来自 HTTP 轮询
未全部验收完成前，系统不得接入真实交易逻辑
