const SKINS_URL = "https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/skins.json";
const CRATES_URL = "https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/crates.json";
const KNIFE_CASE_ICON = "data:image/svg+xml;utf8," + encodeURIComponent(
  "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 256 256'><rect width='256' height='256' rx='22' fill='#161d26'/><rect x='14' y='14' width='228' height='228' rx='16' fill='#1d2733' stroke='#3e5670' stroke-width='4'/><text x='128' y='108' text-anchor='middle' font-size='72'>🔪</text><text x='128' y='164' text-anchor='middle' font-size='18' fill='#f3c56b' font-family='Arial'>KNIFE CASE</text></svg>"
);
const state = { token: null, activeSessionId: null, presets: [], me: null, invitePack: null, campaign: null, skins: [], crates: [] };
const statusEl = document.getElementById("status");

function setStatus(message, isError = false) {
  statusEl.textContent = message;
  statusEl.classList.toggle("is-error", !!isError);
}

function setPre(id, value) {
  document.getElementById(id).textContent = JSON.stringify(value, null, 2);
}

async function copyText(text, label) {
  if (!text) return setStatus(`${label} is empty`, true);
  try {
    await navigator.clipboard.writeText(text);
    setStatus(`${label} copied`);
  } catch {
    setStatus(`Failed to copy ${label}`, true);
  }
}

function updateInviteActions() {
  const pack = state.invitePack || {};
  const inviteLink = pack.invite_url || "";
  const steamLink = pack.steam_invite_url || "";
  const deepLink = pack.telegram_deeplink || "";
  const simulatorLink = inviteLink || "";

  document.getElementById("copyInviteLink").onclick = () => copyText(inviteLink, "Invite link");
  document.getElementById("copySteamLink").onclick = () => copyText(steamLink, "Steam link");
  document.getElementById("copyDeepLink").onclick = () => copyText(deepLink, "Telegram deep link");
  document.getElementById("openSimulator").onclick = () => {
    if (!simulatorLink) return setStatus("Invite link not ready yet", true);
    window.open(simulatorLink, "_blank");
  };
}

function renderCampaign(campaign) {
  const output = document.getElementById("campaignOutput");
  const bar = document.getElementById("campaignProgressBar");
  if (!campaign) {
    output.textContent = JSON.stringify({ message: "No campaign loaded." }, null, 2);
    bar.style.width = "0%";
    return;
  }
  const progress = Number(campaign.progress_percent || 0);
  bar.style.width = `${Math.max(0, Math.min(100, progress))}%`;
  output.textContent = JSON.stringify(campaign, null, 2);
}

function updateRuleRewardMode() {
  const form = document.getElementById("ruleForm");
  const prizeType = form.querySelector("select[name='prize_type']").value;
  const caseWrap = document.getElementById("caseRewardWrap");
  const skinWrap = document.getElementById("skinRewardWrap");
  const hiddenName = form.querySelector("input[name='prize_name']");
  if (prizeType === "skin") {
    caseWrap.style.display = "none";
    skinWrap.style.display = "block";
    const selected = form.querySelector("select[name='skin_prize_name']").value;
    hiddenName.value = selected || "AK-47 | Slate";
  } else {
    caseWrap.style.display = "block";
    skinWrap.style.display = "none";
    const selected = form.querySelector("select[name='case_prize_name']").value;
    hiddenName.value = selected || "Revolution Case";
    renderCasePreview();
  }
}

function renderSkinPreview() {
  const form = document.getElementById("ruleForm");
  const select = form.querySelector("select[name='skin_prize_name']");
  const name = select.value || "";
  const img = document.getElementById("skinPreviewImg");
  const label = document.getElementById("skinPreviewName");
  const skin = state.skins.find((s) => s.name === name);
  label.textContent = skin?.name || "Pick a skin reward";
  img.src = skin?.image || "";
  img.style.visibility = skin?.image ? "visible" : "hidden";
}

function renderCasePreview() {
  const form = document.getElementById("ruleForm");
  const select = form.querySelector("select[name='case_prize_name']");
  const name = select.value || "";
  const img = document.getElementById("casePreviewImg");
  const label = document.getElementById("casePreviewName");
  const lower = name.toLowerCase();
  let icon = "";
  if (lower.includes("knife")) {
    icon = resolveKnifeCaseIcon();
  } else {
    const crate = state.crates.find((c) => String(c?.name || "").toLowerCase() === lower);
    icon = crate?.image || "";
  }
  label.textContent = name || "Pick a case reward";
  img.src = icon;
  img.style.visibility = icon ? "visible" : "hidden";
}

function resolveKnifeCaseIcon() {
  const preferred = ["gamma", "chroma", "spectrum", "danger", "fracture", "shadow"];
  for (const hint of preferred) {
    const c = state.crates.find((it) => String(it?.name || "").toLowerCase().includes(hint) && it?.image);
    if (c?.image) return c.image;
  }
  const any = state.crates.find((it) => it?.image);
  return any?.image || KNIFE_CASE_ICON;
}

function populateSkinOptions() {
  const form = document.getElementById("ruleForm");
  const select = form.querySelector("select[name='skin_prize_name']");
  select.innerHTML = "";
  const fallback = [
    { name: "AK-47 | Slate", image: "" },
    { name: "M4A1-S | Cyrex", image: "" },
    { name: "AWP | Wildfire", image: "" },
  ];
  const source = state.skins.length ? state.skins : fallback;
  for (const skin of source.slice(0, 140)) {
    const option = document.createElement("option");
    option.value = skin.name;
    option.textContent = skin.name;
    select.appendChild(option);
  }
  if (select.options.length > 0) select.value = select.options[0].value;
  renderSkinPreview();
  renderCasePreview();
  updateRuleRewardMode();
}

function updateRoleLabel() {
  const el = document.getElementById("roleLabel");
  const role = state.me?.role || "guest";
  const username = state.me?.username ? ` (${state.me.username})` : "";
  el.textContent = `Role: ${role}${username}`;
}

function updateTopbar() {
  const role = state.me?.role || "guest";
  const username = state.me?.username || "anonymous";
  const sessionLabel = state.activeSessionId ? `Session #${state.activeSessionId}` : "No Active Session";

  document.getElementById("chipAuth").textContent = `${role}: ${username}`;
  document.getElementById("chipSession").textContent = sessionLabel;

  document.getElementById("kpiRole").textContent = role;
  document.getElementById("kpiSession").textContent = state.activeSessionId ? String(state.activeSessionId) : "none";
  document.getElementById("kpiPresets").textContent = String(state.presets.length || 0);
  document.getElementById("logoutBtn").style.display = state.me ? "inline-flex" : "none";
}

function loadToken() {
  state.token = localStorage.getItem("jwt");
}

async function logout() {
  try {
    await api("/api/auth/logout", { method: "POST", body: "{}" });
  } catch {
    // continue local reset even if backend call fails
  }
  localStorage.removeItem("jwt");
  state.token = null;
  state.me = null;
  state.activeSessionId = null;
  state.invitePack = null;
  updateRoleLabel();
  updateTopbar();
  updateInviteActions();
  renderQR(null);
  setPre("streamOutput", { message: "Logged out. Start a new stream after login." });
  setStatus("Logged out.");
  bootstrap();
}

function renderQR(base64Png) {
  const wrap = document.getElementById("qrWrap");
  wrap.innerHTML = "";
  if (!base64Png) return;
  const img = document.createElement("img");
  img.src = `data:image/png;base64,${base64Png}`;
  img.alt = "Invite QR";
  img.className = "qr-image";
  wrap.appendChild(img);
}

function renderPresets(presets) {
  const root = document.getElementById("presetList");
  root.innerHTML = "";
  for (const p of presets) {
    const btn = document.createElement("button");
    btn.className = "ghost preset-btn";
    btn.type = "button";
    btn.textContent = `${p.label} (${p.trigger_type})`;
    btn.title = p.description;
    btn.addEventListener("click", () => {
      const form = document.getElementById("ruleForm");
      form.querySelector("input[name='trigger_type']").value = p.trigger_type;
      setStatus(`Preset selected: ${p.label}`);
    });
    root.appendChild(btn);
  }
}

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;

  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  const text = await response.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = { raw: text }; }
  if (!response.ok) throw new Error(data.error || data.message || data.raw || `HTTP ${response.status}`);
  return data;
}

async function handleRegister(e) {
  e.preventDefault();
  const payload = Object.fromEntries(new FormData(e.target).entries());
  try {
    const data = await api("/api/auth/register", { method: "POST", body: JSON.stringify(payload) });
    state.token = data.token;
    localStorage.setItem("jwt", data.token);
    state.me = data.user;
    updateRoleLabel();
    setStatus(`Registered ${data.user.username}`);
    await bootstrap();
  } catch (err) { setStatus(err.message, true); }
}

async function handleLogin(e) {
  e.preventDefault();
  const payload = Object.fromEntries(new FormData(e.target).entries());
  try {
    const data = await api("/api/auth/login", { method: "POST", body: JSON.stringify(payload) });
    state.token = data.token;
    localStorage.setItem("jwt", data.token);
    state.me = data.user;
    updateRoleLabel();
    setStatus(`Logged in as ${data.user.username}`);
    await bootstrap();
  } catch (err) { setStatus(err.message, true); }
}

async function startStream(e) {
  e.preventDefault();
  const formData = new FormData(e.target);
  const payload = Object.fromEntries(formData.entries());
  payload.send_to_chat = !!payload.send_to_chat;
  try {
    const data = await api("/api/streams/start", { method: "POST", body: JSON.stringify(payload) });
    state.activeSessionId = data.session.id;
    state.invitePack = data;
    document.querySelector("#ruleForm input[name='session_id']").value = String(state.activeSessionId);
    setPre("streamOutput", data);
    renderQR(data.qr_code_png_base64);
    updateInviteActions();
    updateTopbar();
    setStatus(`Stream started. Session #${data.session.id}`);
  } catch (err) {
    if (String(err.message).includes("HTTP 403")) {
      setStatus("Need streamer role. Click 'Enable Streamer Mode' then retry.", true);
      return;
    }
    setStatus(err.message, true);
  }
}

async function loadActiveStream() {
  try {
    const data = await api("/api/streams/me/active");
    setPre("streamOutput", data);
    if (data.session && data.session.id) {
      state.activeSessionId = data.session.id;
      document.querySelector("#ruleForm input[name='session_id']").value = String(state.activeSessionId);
      state.invitePack = data;
    }
    renderQR(data.qr_code_png_base64);
    updateInviteActions();
    updateTopbar();
    await loadCampaignByInvite();
  } catch (err) {
    if (String(err.message).includes("HTTP 403")) return;
    setStatus(err.message, true);
  }
}

async function createCampaign(e) {
  e.preventDefault();
  if (!state.activeSessionId) return setStatus("Start or load active stream first.", true);
  const payload = Object.fromEntries(new FormData(e.target).entries());
  payload.stream_session_id = Number(state.activeSessionId);
  const targetDollars = Number(payload.target_amount_dollars || 0);
  payload.target_amount_dollars = targetDollars;
  payload.target_amount_cents = Math.round(targetDollars * 100);
  try {
    const data = await api("/api/cases", { method: "POST", body: JSON.stringify(payload) });
    setStatus(`Campaign created: #${data.case.id}`);
    await loadCampaignByInvite();
  } catch (err) { setStatus(err.message, true); }
}

function updateCampaignRewardMode() {
  const form = document.getElementById("campaignForm");
  if (!form) return;
  const rewardType = form.querySelector("select[name='reward_item_type']").value;
  const rewardName = form.querySelector("input[name='reward_item_name']");
  if (!rewardName) return;
  if (rewardType === "skin") {
    rewardName.placeholder = "Exact skin name";
    if (!rewardName.value || rewardName.value === "Revolution Case") {
      rewardName.value = "AK-47 | Slate";
    }
    return;
  }
  rewardName.placeholder = "Case name";
  if (!rewardName.value || rewardName.value === "AK-47 | Slate") {
    rewardName.value = "Revolution Case";
  }
}

async function loadCampaignByInvite() {
  const invite = state.invitePack?.session?.invite_code || state.invitePack?.invite_code;
  if (!invite) return renderCampaign(null);
  try {
    const data = await api(`/api/crowdfunding/invite/${encodeURIComponent(invite)}`);
    state.campaign = data.campaign;
    renderCampaign(state.campaign);
  } catch {
    renderCampaign(null);
  }
}

function readRulePayload() {
  const form = document.getElementById("ruleForm");
  updateRuleRewardMode();
  const p = Object.fromEntries(new FormData(form).entries());
  return {
    sessionID: Number(p.session_id || state.activeSessionId),
    ruleID: Number(p.rule_id || 0),
    body: {
      trigger_type: p.trigger_type,
      prize_type: p.prize_type,
      prize_name: p.prize_name,
      prize_cents: Number(p.prize_cents),
      enabled: !!p.enabled,
    }
  };
}

async function createRule(e) {
  e.preventDefault();
  const { sessionID, body } = readRulePayload();
  if (!sessionID) return setStatus("Session ID required", true);
  try {
    await api(`/api/streams/${sessionID}/giveaways`, { method: "POST", body: JSON.stringify(body) });
    setStatus("Rule created.");
    await loadRules();
  } catch (err) { setStatus(err.message, true); }
}

async function updateRule() {
  const { sessionID, ruleID, body } = readRulePayload();
  if (!sessionID || !ruleID) return setStatus("Session ID and Rule ID required", true);
  try {
    await api(`/api/streams/${sessionID}/giveaways/${ruleID}`, { method: "PUT", body: JSON.stringify(body) });
    setStatus("Rule updated.");
    await loadRules();
  } catch (err) { setStatus(err.message, true); }
}

async function deleteRule(ruleID) {
  const sessionID = Number(document.querySelector("#ruleForm input[name='session_id']").value || state.activeSessionId);
  if (!sessionID) return setStatus("Session ID required", true);
  try {
    await api(`/api/streams/${sessionID}/giveaways/${ruleID}`, { method: "DELETE" });
    setStatus(`Rule ${ruleID} deleted.`);
    await loadRules();
  } catch (err) { setStatus(err.message, true); }
}

async function loadRules() {
  const sessionID = Number(document.querySelector("#ruleForm input[name='session_id']").value || state.activeSessionId);
  if (!sessionID) return setStatus("Session ID required", true);
  try {
    const [rules, participants] = await Promise.all([
      api(`/api/streams/${sessionID}/giveaways`),
      api(`/api/streams/${sessionID}/participants`),
    ]);

    const rulesWithActions = (rules.rules || []).map(r => ({ ...r, actions: `delete(${r.id})` }));
    setPre("rulesOutput", { session_id: sessionID, participants: participants.count, rules: rulesWithActions });

    const output = document.getElementById("rulesOutput");
    const parent = output.parentElement;
    const existing = parent.querySelector(".rule-delete-wrap");
    if (existing) existing.remove();
    const controls = document.createElement("div");
    controls.className = "rule-delete-wrap";
    for (const rule of (rules.rules || [])) {
      const btn = document.createElement("button");
      btn.className = "ghost";
      btn.type = "button";
      btn.textContent = `Delete Rule #${rule.id}`;
      btn.addEventListener("click", () => deleteRule(rule.id));
      controls.appendChild(btn);
    }
    parent.appendChild(controls);
  } catch (err) { setStatus(err.message, true); }
}

async function loadPresets() {
  try {
    const data = await api("/api/streams/events/presets");
    state.presets = data.presets || [];
    renderPresets(state.presets);
    updateTopbar();
  } catch (err) { setStatus(err.message, true); }
}

async function loadSkinCatalog() {
  try {
    const response = await fetch(SKINS_URL);
    const skins = await response.json();
    state.skins = (skins || []).filter((s) => s && s.name && s.image);
    populateSkinOptions();
  } catch {
    state.skins = [];
    populateSkinOptions();
  }
}

async function loadCrateCatalog() {
  try {
    const response = await fetch(CRATES_URL);
    const crates = await response.json();
    state.crates = (crates || []).filter((c) => c && c.name && c.image);
  } catch {
    state.crates = [];
  } finally {
    renderCasePreview();
  }
}

async function loadRounds() {
  try {
    const data = await api("/api/lottery/rounds?limit=30");
    setPre("roundsOutput", data.rounds);
  } catch (err) { setStatus(err.message, true); }
}

async function loadEvents() {
  try {
    const data = await api("/api/events?limit=30");
    setPre("eventsOutput", data.events);
    document.getElementById("kpiEvents").textContent = String((data.events || []).length);
  } catch (err) { setStatus(err.message, true); }
}

async function generateFakeEvent(e) {
  e.preventDefault();
  const payload = Object.fromEntries(new FormData(e.target).entries());
  payload.count = Number(payload.count || 1);
  try {
    const data = await api("/api/gsi/fake", { method: "POST", body: JSON.stringify(payload) });
    const n = Number(data.generated_events || data.generated || payload.count || 1);
    setStatus(`Generated ${n} fake ${payload.event_type} event(s).`);
    await Promise.all([loadRounds(), loadEvents()]);
  } catch (err) { setStatus(err.message, true); }
}

async function loadMe() {
  try {
    const data = await api("/api/auth/me");
    state.me = data.user;
    if (!state.token) state.token = localStorage.getItem("jwt");
    updateRoleLabel();
    updateTopbar();
  } catch (err) {
    state.me = null;
    updateRoleLabel();
    updateTopbar();
  }
}

async function enableStreamerMode() {
  if (!state.token) return setStatus("Login first.", true);
  try {
    const data = await api("/api/auth/become-streamer", { method: "POST", body: "{}" });
    state.me = data.user;
    updateRoleLabel();
    updateTopbar();
    setStatus(`Streamer mode enabled for ${data.user.username}`);
  } catch (err) { setStatus(err.message, true); }
}

async function bootstrap() {
  await Promise.all([loadMe(), loadPresets(), loadSkinCatalog(), loadCrateCatalog(), loadRounds(), loadEvents()]);
  if (!state.me) return;
  await Promise.all([loadActiveStream(), loadRules().catch(() => null), loadCampaignByInvite()]);
}

function bind() {
  document.getElementById("registerForm").addEventListener("submit", handleRegister);
  document.getElementById("loginForm").addEventListener("submit", handleLogin);
  document.getElementById("startStreamForm").addEventListener("submit", startStream);
  document.getElementById("ruleForm").addEventListener("submit", createRule);
  document.getElementById("updateRule").addEventListener("click", updateRule);
  document.getElementById("loadActiveStream").addEventListener("click", loadActiveStream);
  document.getElementById("loadRules").addEventListener("click", loadRules);
  document.getElementById("refreshRounds").addEventListener("click", loadRounds);
  document.getElementById("refreshEvents").addEventListener("click", loadEvents);
  document.getElementById("enableStreamer").addEventListener("click", enableStreamerMode);
  document.getElementById("refreshMe").addEventListener("click", loadMe);
  document.getElementById("fakeEventForm").addEventListener("submit", generateFakeEvent);
  document.getElementById("campaignForm").addEventListener("submit", createCampaign);
  document.getElementById("loadCampaignByInvite").addEventListener("click", loadCampaignByInvite);
  const campaignForm = document.getElementById("campaignForm");
  campaignForm.querySelector("select[name='reward_item_type']").addEventListener("change", updateCampaignRewardMode);
  const ruleForm = document.getElementById("ruleForm");
  ruleForm.querySelector("select[name='prize_type']").addEventListener("change", updateRuleRewardMode);
  ruleForm.querySelector("select[name='case_prize_name']").addEventListener("change", () => {
    renderCasePreview();
    updateRuleRewardMode();
  });
  ruleForm.querySelector("select[name='skin_prize_name']").addEventListener("change", () => {
    renderSkinPreview();
    updateRuleRewardMode();
  });
  document.getElementById("logoutBtn").addEventListener("click", logout);
  document.getElementById("goToStream").addEventListener("click", () => window.switchViewById("view-stream"));
  document.getElementById("goToMonitor").addEventListener("click", () => window.switchViewById("view-monitor"));
}

function init() {
  loadToken();
  bind();
  updateRuleRewardMode();
  updateCampaignRewardMode();
  updateInviteActions();
  updateTopbar();
  bootstrap();
}

init();
