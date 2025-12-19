"use strict";

function $(id) { return document.getElementById(id); }

async function apiGet(path) {
  const res = await fetch(path, { credentials: "include" });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function apiPost(path, body) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

function setMsg(id, text, ok) {
  const el = $(id);
  el.textContent = text || "";
  el.className = "msg " + (ok ? "ok" : "bad");
  if (text) setTimeout(() => (el.textContent = ""), 2500);
}

function bindRange(rangeId, valId) {
  const r = $(rangeId);
  const v = $(valId);
  const sync = () => (v.textContent = r.value);
  r.addEventListener("input", sync);
  sync();
}

async function loadAPIKeys() {
  const data = await apiGet("/api/apikey");
  const keys = (data.apiKeys || []).join("\n");
  $("apikeys").value = keys;
}

async function saveAPIKeys() {
  const raw = $("apikeys").value || "";
  const keys = raw.split("\n").map(s => s.trim()).filter(Boolean);
  try {
    const out = await apiPost("/api/apikey", { apiKeys: keys });
    $("apikeys").value = (out.apiKeys || []).join("\n");
    setMsg("msg-apikey", "已保存", true);
  } catch (e) {
    setMsg("msg-apikey", "保存失败: " + e.message, false);
  }
}

async function loadRules() {
  const r = await apiGet("/api/rules");

  $("on-enabled").checked = !!r.on?.enabled;
  $("off-enabled").checked = !!r.off?.enabled;
  $("hit-enabled").checked = !!r.hit?.enabled;

  $("on-threshold").value = (r.on?.threshold ?? 5);
  $("off-threshold").value = (r.off?.threshold ?? 5);
  $("hit-offset").value = (r.hit?.offset ?? 1);
  $("hit-expect").value = (r.hit?.expect ?? "ON");

  $("on-threshold-val").textContent = $("on-threshold").value;
  $("off-threshold-val").textContent = $("off-threshold").value;
  $("hit-offset-val").textContent = $("hit-offset").value;
}

let currentJudgeRule = null;

function judgeLabel(v) {
  switch (v) {
    case "LUCKY": return "幸运";
    case "BIGSMALL": return "大小";
    case "ODDEVEN": return "单双";
    default: return v || "";
  }
}

async function loadJudge() {
  try {
    const out = await apiGet("/api/judge");
    currentJudgeRule = out.rule || "LUCKY";
    if ($("judge-rule")) $("judge-rule").value = currentJudgeRule;
  } catch (e) {
    // ignore on older versions
  }
}

function showJudgeModal(fromRule, toRule) {
  const modal = $("judge-modal");
  $("judge-modal-text").textContent =
    `你将从【${judgeLabel(fromRule)}】切换到【${judgeLabel(toRule)}】。\n\n` +
    `切换会立即中断所有状态机并清空计数器，且会自动停止（需手动重新启用）。`;
  $("judge-ack").checked = false;
  $("judge-confirm").disabled = true;
  modal.classList.add("show");
  modal.setAttribute("aria-hidden", "false");
}

function hideJudgeModal() {
  const modal = $("judge-modal");
  modal.classList.remove("show");
  modal.setAttribute("aria-hidden", "true");
}

async function applyJudgeRule() {
  const toRule = $("judge-rule").value;
  const fromRule = currentJudgeRule || "LUCKY";

  if (toRule === fromRule) {
    setMsg("msg-judge", "当前已是该判定规则", true);
    return;
  }

  const ok1 = window.confirm("切换 ON/OFF 判定规则会立即中断所有状态机并清空计数器，并自动停止（需手动重新启用）。\n\n继续吗？");
  if (!ok1) return;

  showJudgeModal(fromRule, toRule);

  const onAck = () => {
    $("judge-confirm").disabled = !$("judge-ack").checked;
  };
  $("judge-ack").onchange = onAck;
  onAck();

  $("judge-cancel").onclick = () => hideJudgeModal();

  $("judge-confirm").onclick = async () => {
    try {
      await apiPost("/api/judge", { rule: toRule, confirm: true, ackStop: true });
      currentJudgeRule = toRule;
      hideJudgeModal();
      setMsg("msg-judge", `已切换为【${judgeLabel(toRule)}】，所有状态机已停止并清空计数器`, true);
      // reload rules because backend will disable them
      await loadRules();
      await loadStatus();
    } catch (e) {
      hideJudgeModal();
      setMsg("msg-judge", "切换失败: " + e.message, false);
    }
  };
}

async function saveRules() {
  const body = {
    on: {
      enabled: $("on-enabled").checked,
      threshold: parseInt($("on-threshold").value, 10),
    },
    off: {
      enabled: $("off-enabled").checked,
      threshold: parseInt($("off-threshold").value, 10),
    },
    hit: {
      enabled: $("hit-enabled").checked,
      offset: parseInt($("hit-offset").value, 10),
      expect: $("hit-expect").value,
    }
  };

  try {
    await apiPost("/api/rules", body);
    setMsg("msg-rules", "已保存", true);
  } catch (e) {
    setMsg("msg-rules", "保存失败: " + e.message, false);
  }
}

function renderStatus(st) {
  $("sys-status").textContent = st.listening ? "Listening" : "Idle";
  $("ws-reconnect").textContent = String(st.reconnects ?? 0);
  $("last-height").textContent = st.lastHeight ? String(st.lastHeight) : "-";
  $("last-time").textContent = st.lastTimeISO || "-";
}

async function loadStatus() {
  try {
    const st = await apiGet("/api/status");
    renderStatus(st);
  } catch (e) {
    // likely not logged in
  }
}

function startSSE() {
  const es = new EventSource("/sse/status");
  es.addEventListener("status", (ev) => {
    try {
      const st = JSON.parse(ev.data);
      renderStatus(st);
    } catch {}
  });
  es.onerror = () => {
    // EventSource will auto-reconnect
  };
}

function init() {
  bindRange("on-threshold", "on-threshold-val");
  bindRange("off-threshold", "off-threshold-val");
  bindRange("hit-offset", "hit-offset-val");

  $("btn-save-apikey").addEventListener("click", saveAPIKeys);
  $("btn-save-rules").addEventListener("click", saveRules);
  if ($("judge-apply")) $("judge-apply").addEventListener("click", applyJudgeRule);

  loadAPIKeys();
  loadRules();
  loadJudge();
  loadStatus();
  startSSE();

  setInterval(loadStatus, 3000);
}

window.addEventListener("DOMContentLoaded", init);