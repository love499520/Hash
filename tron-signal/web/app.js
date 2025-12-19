// web/app.js
// Tron Signal Web UI Logic
// 所有时间显示统一为北京时间（UTC+8）
// 所有规则阈值统一使用滚动条 0–20

const API = {
  STATUS: "/api/status",
  RULES_GET: "/api/rules",
  RULES_SET: "/api/rules",
  APIKEY_GET: "/api/apikey",
  APIKEY_SET: "/api/apikey",
  SSE_STATUS: "/sse/status"
};

function $(id) {
  return document.getElementById(id);
}

// ---------- 时间处理 ----------
function formatBeijingTime(utcTs) {
  if (!utcTs) return "-";
  const d = new Date(utcTs);
  // 转为北京时间
  const bj = new Date(d.getTime() + 8 * 3600 * 1000);
  return bj.toISOString().replace("T", " ").substring(0, 19);
}

// ---------- API 基础 ----------
async function apiGet(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

async function apiPost(url, data) {
  const r = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data)
  });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

// ---------- 状态 ----------
async function loadStatus() {
  try {
    const s = await apiGet(API.STATUS);
    $("status-block").innerText = s.connected ? "WS 已连接" : "WS 未连接";
    $("last-block").innerText = s.last_block || "-";
    $("last-time").innerText = formatBeijingTime(s.last_block_time);
    $("ws-reconnect").innerText = s.ws_reconnects;
  } catch (e) {
    console.error("loadStatus error:", e);
  }
}

// ---------- API Key ----------
async function loadApiKey() {
  try {
    const r = await apiGet(API.APIKEY_GET);
    $("apikey-input").value = r.api_key || "";
  } catch (e) {
    console.error("loadApiKey error:", e);
  }
}

async function saveApiKey() {
  const key = $("apikey-input").value.trim();
  await apiPost(API.APIKEY_SET, { api_key: key });
  alert("API Key 已保存");
}

// ---------- 规则 ----------
function bindSlider(sliderId, valueId) {
  const s = $(sliderId);
  const v = $(valueId);
  v.innerText = s.value;
  s.oninput = () => (v.innerText = s.value);
}

async function loadRules() {
  try {
    const r = await apiGet(API.RULES_GET);

    $("on-enabled").checked = r.on.enabled;
    $("on-threshold").value = r.on.threshold;
    $("on-threshold-val").innerText = r.on.threshold;

    $("off-enabled").checked = r.off.enabled;
    $("off-threshold").value = r.off.threshold;
    $("off-threshold-val").innerText = r.off.threshold;

    $("hit-enabled").checked = r.hit.enabled;
    $("hit-offset").value = r.hit.offset;
    $("hit-offset-val").innerText = r.hit.offset;
  } catch (e) {
    console.error("loadRules error:", e);
  }
}

async function saveRules() {
  const payload = {
    on: {
      enabled: $("on-enabled").checked,
      threshold: Number($("on-threshold").value)
    },
    off: {
      enabled: $("off-enabled").checked,
      threshold: Number($("off-threshold").value)
    },
    hit: {
      enabled: $("hit-enabled").checked,
      offset: Number($("hit-offset").value)
    }
  };
  await apiPost(API.RULES_SET, payload);
  alert("规则已保存");
}

// ---------- SSE ----------
function startSSE() {
  const es = new EventSource(API.SSE_STATUS);
  es.onmessage = (e) => {
    try {
      const s = JSON.parse(e.data);
      $("last-block").innerText = s.last_block || "-";
      $("last-time").innerText = formatBeijingTime(s.last_block_time);
    } catch {}
  };
  es.onerror = () => {
    console.warn("SSE disconnected");
  };
}

// ---------- 初始化 ----------
window.onload = () => {
  // 绑定滑动条
  bindSlider("on-threshold", "on-threshold-val");
  bindSlider("off-threshold", "off-threshold-val");
  bindSlider("hit-offset", "hit-offset-val");

  $("save-apikey").onclick = saveApiKey;
  $("save-rules").onclick = saveRules;

  loadApiKey();
  loadRules();
  loadStatus();
  startSSE();

  setInterval(loadStatus, 3000);
};
