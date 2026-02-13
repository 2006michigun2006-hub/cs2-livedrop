const statusEl = document.getElementById("status");
const inviteLabel = document.getElementById("inviteLabel");
const steamJoinLink = document.getElementById("steamJoinLink");
const inventoryGrid = document.getElementById("inventoryGrid");
const logoutBtn = document.getElementById("logoutBtn");
const authStatePill = document.getElementById("authStatePill");
const profileNameEl = document.getElementById("profileName");
const profileSteamEl = document.getElementById("profileSteam");
const profileBalanceEl = document.getElementById("profileBalance");

const caseWindow = document.getElementById("caseWindow");
const caseReel = document.getElementById("caseReel");
const openStatus = document.getElementById("openStatus");
const campaignStatusHint = document.getElementById("campaignStatusHint");
const campaignRewardIcon = document.getElementById("campaignRewardIcon");
const campaignRewardType = document.getElementById("campaignRewardType");
const campaignRewardName = document.getElementById("campaignRewardName");
const campaignProgressBar = document.getElementById("campaignProgressBar");
const campaignProgressText = document.getElementById("campaignProgressText");
const campaignDonateAmount = document.getElementById("campaignDonateAmount");
const campaignDonateBtn = document.getElementById("campaignDonateBtn");
const campaignChanceText = document.getElementById("campaignChanceText");
const crowdfundingSection = document.getElementById("crowdfundingSection");
const specialDropOverlay = document.getElementById("specialDropOverlay");
const specialDropSkull = document.getElementById("specialDropSkull");
const eventToast = document.getElementById("eventToast");

const CARD_WIDTH = 192;
const WINNER_INDEX = 40;
const TOTAL_CARDS = 50;

const CRATES_URL = "https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/crates.json";
const SKINS_URL = "https://raw.githubusercontent.com/ByMykel/CSGO-API/main/public/api/en/skins.json";
const SPIN_AUDIO_URL = "https://raw.githubusercontent.com/poofy25/Case-Simulator/main/Audio/CSGO%20Case%20Opening%20Sound%20Effect.mp3";
const SPECIAL_DROP_AUDIO_URL = "https://www.myinstants.com/media/sounds/4092-mango-phonk.mp3";
const SKULL_FALLBACK_URL = "data:image/svg+xml;utf8," + encodeURIComponent(
  "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 300 300'><rect width='300' height='300' fill='black'/><text x='150' y='180' text-anchor='middle' font-size='160' fill='white'>☠</text></svg>"
);
const RARITY_SOUNDS = {
  consumer:   { name: "Perdyozh",    url: "https://www.myinstants.com/media/sounds/perdiozh.mp3" },
  industrial: { name: "Perdyozh",    url: "https://www.myinstants.com/media/sounds/perdiozh.mp3" },
  milspec:    { name: "Bruh",        url: "https://www.myinstants.com/media/sounds/movie_1.mp3" },
  restricted: { name: "Yippee",      url: "https://www.myinstants.com/media/sounds/yippeeeeeeeeeeeeee.mp3" },
  classified: { name: "YAAAAAAAAY",  url: "https://www.myinstants.com/media/sounds/kids-saying-yay-sound-effect_3.mp3" },
  covert:     { name: "Mango Phonk", url: SPECIAL_DROP_AUDIO_URL },
  gold:       { name: "Mango Phonk", url: SPECIAL_DROP_AUDIO_URL },
};

const state = {
  token: null,
  invite: "",
  crates: [],
  skins: [],
  caseByName: new Map(),
  skinByName: new Map(),
  spinAudio: null,
  specialDropAudio: null,
  specialDropTimer: null,
  toastTimer: null,
  raritySounds: {},
  campaign: null,
  lastCampaignID: null,
  lastCampaignStatus: "",
  lastClosedToastKey: "",
};

function setStatus(message, isError = false) {
  statusEl.textContent = message;
  statusEl.style.color = isError ? "#ff6f61" : "#8a9bad";
}

function formatUSD(cents) {
  const n = Number(cents || 0);
  return `$${(n / 100).toFixed(2)}`;
}

function showToast(message) {
  if (!eventToast) return;
  eventToast.textContent = message;
  eventToast.style.display = "block";
  if (state.toastTimer) clearTimeout(state.toastTimer);
  state.toastTimer = setTimeout(() => {
    eventToast.style.display = "none";
  }, 3600);
}

function setProfile(user) {
  if (!user) {
    authStatePill.textContent = "Guest";
    profileNameEl.textContent = "Not logged in";
    profileSteamEl.textContent = "Steam ID: -";
    profileBalanceEl.textContent = "$0.00";
    logoutBtn.style.display = "none";
    steamJoinLink.style.display = "inline-flex";
    renderCampaign(null);
    return;
  }

  authStatePill.textContent = user.role || "viewer";
  profileNameEl.textContent = user.username || "Steam User";
  profileSteamEl.textContent = `Steam ID: ${user.steam_id || "-"}`;
  profileBalanceEl.textContent = formatUSD(user.balance_cents);
  logoutBtn.style.display = "inline-flex";
  steamJoinLink.style.display = user.steam_id ? "none" : "inline-flex";
}

function parseQuery() {
  const params = new URLSearchParams(window.location.search);
  state.invite = params.get("invite") || "";

  const token = params.get("token");
  if (token) {
    localStorage.setItem("jwt", token);
  }
  state.token = localStorage.getItem("jwt");

  if (state.invite) {
    inviteLabel.textContent = `Invite: ${state.invite}`;
    const href = `/api/auth/steam/login?invite=${encodeURIComponent(state.invite)}`;
    steamJoinLink.href = href;
  } else {
    inviteLabel.textContent = "Invite code missing";
    steamJoinLink.href = "/api/auth/steam/login";
  }

  const inviteJoined = params.get("invite_joined");
  if (inviteJoined === "1") {
    setStatus("Steam login success. Joined stream giveaway pool.");
  } else if (inviteJoined === "0") {
    setStatus("Steam login success, but invite join failed.", true);
  }

  const next = state.invite ? `/simulator.html?invite=${encodeURIComponent(state.invite)}` : "/simulator.html";
  window.history.replaceState({}, "", next);
}

async function api(path, options = {}) {
  const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
  });

  const text = await response.text();
  let data = {};
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = { raw: text };
  }

  if (!response.ok) {
    throw new Error(data.error || data.message || data.raw || `HTTP ${response.status}`);
  }
  return data;
}

async function loadCS2Data() {
  try {
    const [cratesRaw, skinsRaw] = await Promise.all([
      fetch(CRATES_URL).then((r) => r.json()),
      fetch(SKINS_URL).then((r) => r.json()),
    ]);

    state.crates = cratesRaw || [];
    state.skins = skinsRaw || [];
    state.caseByName.clear();
    state.skinByName.clear();

    for (const c of state.crates) {
      if (c && c.name) state.caseByName.set(String(c.name).toLowerCase(), c);
    }
    for (const s of state.skins) {
      if (s && s.name) state.skinByName.set(String(s.name).toLowerCase(), s);
    }

    console.log(`Loaded ${state.crates.length} crates and ${state.skins.length} skins`);
  } catch (err) {
    console.error("Failed to load CS2 data:", err);
    throw err;
  }
}

async function ensureJoined() {
  if (!state.invite) return;
  try {
    await api(`/api/streams/join/${encodeURIComponent(state.invite)}`, { method: "POST", body: "{}" });
  } catch {
    // user may already be joined or no active stream
  }
}

function resolveItemImage(item) {
  if (!item || !item.name) return "";
  const key = String(item.name).toLowerCase();
  if (key.includes("knife fever case") || key.includes("knife case")) return resolveKnifeCaseIcon();
  const skin = state.skinByName.get(key);
  if (skin && skin.image) return skin.image;
  const crate = state.caseByName.get(key);
  if (crate && crate.image) return crate.image;
  return "";
}

function ensureImageFallback(img) {
  if (!img) return;
  img.onerror = () => {
    if (img.parentElement === campaignRewardIcon?.parentElement) {
      campaignRewardIcon.style.visibility = "hidden";
    }
    img.removeAttribute("src");
    img.style.display = "none";
  };
}

function resolveKnifeCaseIcon() {
  const preferred = ["gamma", "chroma", "spectrum", "danger", "fracture", "shadow"];
  for (const hint of preferred) {
    const c = state.crates.find((it) => String(it?.name || "").toLowerCase().includes(hint) && it?.image);
    if (c?.image) return c.image;
  }
  const any = state.crates.find((it) => it?.image);
  return any?.image || "";
}

function resolveRarityColor(item) {
  let rarity = (item?.rarity || "").toLowerCase();
  if (!rarity && item?.name) {
    const skin = state.skinByName.get(String(item.name).toLowerCase());
    if (skin && skin.rarity && skin.rarity.name) rarity = skin.rarity.name.toLowerCase();
  }

  if (rarity.includes("consumer")) return "var(--rarity-consumer)";
  if (rarity.includes("industrial")) return "var(--rarity-industrial)";
  if (rarity.includes("mil-spec") || rarity.includes("milspec")) return "var(--rarity-milspec)";
  if (rarity.includes("restricted")) return "var(--rarity-restricted)";
  if (rarity.includes("classified")) return "var(--rarity-classified)";
  if (rarity.includes("covert") || rarity.includes("extraordinary")) return "var(--rarity-covert)";
  if (rarity.includes("contraband") || rarity.includes("gold")) return "var(--rarity-gold)";
  return "var(--rarity-milspec)";
}

function renderInventory(items) {
  inventoryGrid.innerHTML = "";

  if (!items.length) {
    inventoryGrid.innerHTML = "<p style='grid-column:1/-1;text-align:center;color:#68788a;'>No rewards yet. Wait for stream triggers.</p>";
    return;
  }

  for (const item of items) {
    const card = document.createElement("article");
    card.className = "inv-card";
    card.style.borderBottom = `2px solid ${resolveRarityColor(item)}`;

    const image = resolveItemImage(item);
    if (image) {
      const img = document.createElement("img");
      img.src = image;
      img.alt = item.name;
      img.className = "inv-img";
      ensureImageFallback(img, item.name);
      card.appendChild(img);
    }

    const title = document.createElement("h3");
    title.textContent = item.name;

    const meta = document.createElement("p");
    meta.textContent = `${item.status} | market ${formatUSD(item.price_cents)}`;

    card.appendChild(title);
    card.appendChild(meta);

    const actions = document.createElement("div");
    actions.className = "inv-actions";

    if (item.item_type === "case" && item.status === "unopened") {
      const openBtn = document.createElement("button");
      openBtn.textContent = "Open Case";
      openBtn.className = "inv-btn-open";
      openBtn.addEventListener("click", () => openCase(item));
      actions.appendChild(openBtn);
    }

    if (item.status === "available" || item.status === "unopened") {
      const sellBtn = document.createElement("button");
      sellBtn.textContent = `Sell ${formatUSD(item.price_cents)}`;
      sellBtn.className = "inv-btn-sell";
      sellBtn.addEventListener("click", () => sellItem(item));
      actions.appendChild(sellBtn);
    }

    if (actions.children.length === 1) {
      actions.classList.add("single");
    }
    if (actions.children.length > 0) {
      card.appendChild(actions);
    }
    inventoryGrid.appendChild(card);
  }
}

function renderCampaign(view) {
  if (!view || !view.case || view.case.status !== "open") {
    const closedKey = view?.case ? `${view.case.id}:${view.case.status}` : "";
    if (
      view?.case &&
      view.case.status !== "open" &&
      closedKey &&
      state.lastClosedToastKey !== closedKey
    ) {
      showToast("Crowdfunding goal met. Reward distributed and campaign closed.");
      state.lastClosedToastKey = closedKey;
    }
    state.campaign = null;
    crowdfundingSection.style.display = "none";
    campaignStatusHint.textContent = "Waiting for active campaign";
    campaignRewardType.textContent = "Reward";
    campaignRewardName.textContent = "No active campaign";
    campaignRewardIcon.src = "";
    campaignRewardIcon.style.visibility = "hidden";
    campaignProgressBar.style.width = "0%";
    campaignProgressText.textContent = "0 / 0";
    campaignChanceText.textContent = "Your chance: 0%";
    campaignDonateBtn.disabled = true;
    state.lastCampaignStatus = view?.case?.status || "";
    return;
  }

  state.campaign = view;
  crowdfundingSection.style.display = "block";
  const c = view.case;
  const displayName = c.reward_item_name;
  const icon = resolveItemImage({ name: c.reward_item_name, item_type: c.reward_item_type });
  campaignRewardIcon.src = icon || "";
  campaignRewardIcon.style.visibility = icon ? "visible" : "hidden";
  ensureImageFallback(campaignRewardIcon, c.reward_item_name);
  campaignRewardType.textContent = `${c.reward_item_type} reward`;
  campaignRewardName.textContent = displayName || "Crowdfunding Reward";

  campaignStatusHint.textContent = `Status: ${c.status} | Contributors: ${view.total_contributors}`;
  campaignProgressBar.style.width = `${Math.max(0, Math.min(100, Number(view.progress_percent || 0)))}%`;
  campaignProgressText.textContent = `${formatUSD(view.total_raised_cents)} raised / ${formatUSD(c.target_amount_cents)} target | ${formatUSD(view.left_cents)} left`;
  campaignChanceText.textContent = `Your chance: ${(view.my_chance_percent || 0).toFixed(2)}% (${formatUSD(view.my_contribution_cents)} donated)`;
  campaignDonateBtn.disabled = c.status !== "open";

  if (state.lastCampaignID !== c.id) {
    showToast(`New crowdfunding event: ${c.title} (${campaignRewardType.textContent}).`);
  }
  state.lastCampaignID = c.id;
  state.lastCampaignStatus = c.status;
}

async function loadCampaign() {
  if (!state.token || !state.invite) return;
  try {
    const data = await api(`/api/crowdfunding/invite/${encodeURIComponent(state.invite)}`);
    renderCampaign(data.campaign);
  } catch {
    renderCampaign(null);
  }
}

async function donateCampaign() {
  if (!state.campaign?.case?.id) return setStatus("No active campaign", true);
  const amountDollars = Number(campaignDonateAmount.value || 0);
  if (amountDollars <= 0) return setStatus("Donation amount must be positive", true);
  const amountCents = Math.round(amountDollars * 100);
  try {
    const data = await api(`/api/cases/${state.campaign.case.id}/contribute`, {
      method: "POST",
      body: JSON.stringify({ amount_dollars: amountDollars, amount_cents: amountCents }),
    });
    setStatus(`Donated ${formatUSD(amountCents)} to campaign.`);
    showToast(`Donation accepted: ${formatUSD(amountCents)}.`);
    if (data.reward_item) {
      setStatus(`Campaign completed! Winner received ${data.reward_item.name}.`);
      showToast(`Goal reached. Winner reward: ${data.reward_item.name}.`);
    }
    await Promise.all([loadProfile(), loadCampaign(), loadInventory(), ensureJoined()]);
  } catch (err) {
    setStatus(err.message, true);
  }
}

async function loadInventory() {
  try {
    const data = await api("/api/inventory/me?limit=100");
    renderInventory(data.items || []);
  } catch (err) {
    setStatus(err.message, true);
  }
}

async function loadProfile() {
  try {
    const data = await api("/api/auth/me");
    const user = data.user;
    setProfile(user);
    if (!state.token) {
      state.token = localStorage.getItem("jwt") || null;
    }
    return true;
  } catch {
    setProfile(null);
    return false;
  }
}

function createReelCard(itemData) {
  const card = document.createElement("div");
  
  const rarityClass = resolveRarityKey(itemData.rarity);
  card.className = `cs2-card rarity-${rarityClass}`;

  const bg = document.createElement("div");
  bg.className = "cs2-card-bg";
  card.appendChild(bg);

  const imgWrapper = document.createElement("div");
  imgWrapper.className = "cs2-img-wrapper";
  if (itemData.image) {
    const img = document.createElement("img");
    img.src = itemData.image;
    ensureImageFallback(img, itemData.name);
    imgWrapper.appendChild(img);
  }
  card.appendChild(imgWrapper);

  const info = document.createElement("div");
  info.className = "cs2-info";

  let weaponName = "Unknown";
  let skinName = itemData.name;
  if (itemData.name && itemData.name.includes("|")) {
    const parts = itemData.name.split("|");
    weaponName = parts[0].trim();
    skinName = parts[1].trim();
  }

  const wDiv = document.createElement("div");
  wDiv.className = "cs2-weapon";
  wDiv.textContent = weaponName;

  const sDiv = document.createElement("div");
  sDiv.className = "cs2-skin";
  sDiv.textContent = skinName;

  info.appendChild(wDiv);
  info.appendChild(sDiv);
  card.appendChild(info);

  const bar = document.createElement("div");
  bar.className = "cs2-rarity-bar";
  card.appendChild(bar);

  return card;
}

function loadAudio() {
  if (!state.spinAudio) {
    state.spinAudio = new Audio(SPIN_AUDIO_URL);
    state.spinAudio.crossOrigin = "anonymous";
    state.spinAudio.load();

    const AudioContext = window.AudioContext || window.webkitAudioContext;
    if (AudioContext) {
      const ctx = new AudioContext();
      const source = ctx.createMediaElementSource(state.spinAudio);
      const gain = ctx.createGain();
      gain.gain.value = 12.0; 
      source.connect(gain);
      gain.connect(ctx.destination);
      state.spinAudioCtx = ctx;
    }
  }

  if (!Object.keys(state.raritySounds).length) {
    const audioCache = {};
    for (const [key, entry] of Object.entries(RARITY_SOUNDS)) {
      if (!audioCache[entry.url]) {
        const audio = new Audio(entry.url);
        audio.volume = 0.8;
        audio.load();
        audioCache[entry.url] = audio;
      }
      state.raritySounds[key] = audioCache[entry.url];
    }
  }

  if (!state.specialDropAudio) {
    state.specialDropAudio = new Audio(SPECIAL_DROP_AUDIO_URL);
    state.specialDropAudio.volume = 0.85;
    state.specialDropAudio.load();
    state.specialDropAudio.addEventListener("ended", clearSpecialDropFx);
  }
}

function playRevealSound(rarity) {
  const key = resolveRarityKey(rarity);
  const audio = state.raritySounds[key] || state.raritySounds.milspec;
  if (!audio) return;
  audio.currentTime = 0;
  audio.play().catch(() => {});
}

function resolveRarityKey(rarity) {
  const raw = typeof rarity === "object" && rarity !== null ? (rarity.name || "") : (rarity || "");
  const r = String(raw).toLowerCase();
  if (r.includes("consumer")) return "consumer";
  if (r.includes("industrial")) return "industrial";
  if (r.includes("mil-spec") || r.includes("milspec")) return "milspec";
  if (r.includes("restricted")) return "restricted";
  if (r.includes("classified")) return "classified";
  if (r.includes("covert") || r.includes("extraordinary")) return "covert";
  if (r.includes("contraband") || r.includes("gold")) return "gold";
  return "milspec";
}

function isBestDropRarity(rarity) {
  const k = resolveRarityKey(rarity);
  return k === "covert" || k === "gold";
}

function clearSpecialDropFx() {
  document.body.classList.remove("drop-bw");
  specialDropOverlay.classList.remove("active");
  if (state.specialDropTimer) {
    clearTimeout(state.specialDropTimer);
    state.specialDropTimer = null;
  }
}

function playSpecialDropFx() {
  if (!state.specialDropAudio) return;
  clearSpecialDropFx();
  if (specialDropSkull) {
    specialDropSkull.src = "https://pngfre.com/wp-content/uploads/1000117573-269x300.png";
    specialDropSkull.style.display = "block";
  }
  document.body.classList.add("drop-bw");
  specialDropOverlay.classList.add("active");

  state.specialDropAudio.currentTime = 0;
  state.specialDropAudio.play().catch(() => {
    state.specialDropTimer = setTimeout(clearSpecialDropFx, 5000);
  });
}

async function openCase(caseItem) {
  try {
    openStatus.textContent = "Opening...";
    openStatus.style.color = "#e4ae39";
    caseWindow.classList.add("active");
    caseReel.innerHTML = "";
    caseReel.style.transition = "none";
    caseReel.style.transform = "translateX(0px)";

    const caseData = state.caseByName.get(String(caseItem.name).toLowerCase());
    const pool =
      caseData && caseData.contains && caseData.contains.length
        ? caseData.contains
        : state.skins.slice(0, 120);

    const data = await api(`/api/inventory/open/${caseItem.id}`, { method: "POST", body: "{}" });
    const winnerDrop = data.drop;

    const reelItems = [];
    for (let i = 0; i < TOTAL_CARDS; i++) {
      if (i === WINNER_INDEX) {
        const winnerRarity = winnerDrop.rarity?.name || winnerDrop.rarity || "Mil-Spec Grade";
        reelItems.push({ name: winnerDrop.name, image: resolveItemImage(winnerDrop), rarity: winnerRarity });
      } else {
        const randomItem = pool[Math.floor(Math.random() * pool.length)] || {};
        const itemRarity = randomItem.rarity?.name || randomItem.rarity || "Mil-Spec Grade";
        reelItems.push({
          name: randomItem.name || "Unknown",
          image: randomItem.image || "",
          rarity: itemRarity,
        });
      }
    }

    reelItems.forEach((item) => caseReel.appendChild(createReelCard(item)));
    caseReel.offsetHeight;

    const jitter = (Math.random() - 0.5) * (CARD_WIDTH * 0.7);
    const winnerOffset = WINNER_INDEX * CARD_WIDTH;
    const finalPosition = -(winnerOffset) + caseWindow.offsetWidth / 2 - CARD_WIDTH / 2 + jitter;

    const duration = 6000;
    caseReel.style.transition = `transform ${duration}ms cubic-bezier(0.15, 0, 0.20, 1)`;
    caseReel.style.transform = `translateX(${finalPosition}px)`;

    if (state.spinAudio) {
      if (state.spinAudioCtx && state.spinAudioCtx.state === "suspended") {
        state.spinAudioCtx.resume();
      }
      state.spinAudio.currentTime = 0;
      state.spinAudio.play().catch(() => {});
    }

    setTimeout(async () => {
      if (isBestDropRarity(winnerDrop.rarity)) {
        playSpecialDropFx();
      } else {
        playRevealSound(winnerDrop.rarity);
      }
      openStatus.textContent = `You got: ${winnerDrop.name}`;
      openStatus.style.color = "#fff";
      await loadInventory();
    }, duration);
  } catch (err) {
    setStatus(err.message, true);
    openStatus.textContent = "Error opening case.";
  }
}

async function logout() {
  try {
    await api("/api/auth/logout", { method: "POST", body: "{}" });
  } catch {
    // still clear local token
  }
  localStorage.removeItem("jwt");
  state.token = null;
  setProfile(null);
  inventoryGrid.innerHTML = "";
  setStatus("Logged out.");
}

async function sellItem(item) {
  try {
    const data = await api(`/api/inventory/sell/${item.id}`, { method: "POST", body: "{}" });
    setStatus(`Sold ${item.name} for ${formatUSD(data.credited_cents)}. New balance: ${formatUSD(data.balance_cents)}.`);
    await loadProfile();
    await loadInventory();
  } catch (err) {
    setStatus(err.message, true);
  }
}

async function init() {
  parseQuery();
  loadAudio();
  if (specialDropSkull) {
    specialDropSkull.onerror = () => {
      specialDropSkull.src = SKULL_FALLBACK_URL;
    };
  }

  try {
    await loadCS2Data();
  } catch (err) {
    setStatus(`Failed to load CS2 catalog: ${err.message}`, true);
  }

  const authed = await loadProfile();
  if (authed) {
    await ensureJoined();
    await Promise.all([loadInventory(), loadCampaign()]);
    setInterval(() => {
      if (!openStatus.textContent || openStatus.textContent.startsWith("You got")) {
        loadInventory();
      }
      loadCampaign();
    }, 10000);
  }
}

document.getElementById("refreshInventory").addEventListener("click", loadInventory);
logoutBtn.addEventListener("click", logout);
campaignDonateBtn.addEventListener("click", donateCampaign);

init();
