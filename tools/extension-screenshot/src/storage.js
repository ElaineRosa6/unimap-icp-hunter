export async function saveSessionToken(token, expiresInSeconds) {
  const expireAt = Date.now() + Math.max(1, expiresInSeconds) * 1000;
  await chrome.storage.local.set({ bridgeToken: token, bridgeTokenExpireAt: expireAt });
}

export async function loadSessionToken() {
  const data = await chrome.storage.local.get(["bridgeToken", "bridgeTokenExpireAt"]);
  return {
    token: data.bridgeToken || "",
    expireAt: data.bridgeTokenExpireAt || 0
  };
}

export function isTokenExpired(expireAt) {
  return !expireAt || Date.now() >= expireAt;
}

export async function saveRuntimeState(state) {
  const existing = await chrome.storage.local.get(["bridgeRuntimeState"]);
  const merged = { ...(existing.bridgeRuntimeState || {}), ...state };
  await chrome.storage.local.set({ bridgeRuntimeState: merged });
}

export async function saveLastError(err) {
  await saveRuntimeState({
    last_error: String(err || ""),
    last_error_at: Date.now()
  });
}

// API Base URL storage
const DEFAULT_API_BASE_URL = "http://127.0.0.1:8448";

export async function saveAPIBaseURL(url) {
  await chrome.storage.local.set({ apiBaseURL: url });
}

export async function loadAPIBaseURL() {
  const data = await chrome.storage.local.get(["apiBaseURL"]);
  return data.apiBaseURL || DEFAULT_API_BASE_URL;
}
