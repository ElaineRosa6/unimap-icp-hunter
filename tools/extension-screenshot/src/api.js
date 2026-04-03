import { loadAPIBaseURL } from "./storage.js";

async function baseURL() {
  return await loadAPIBaseURL();
}

function toHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256Hex(text) {
  const data = new TextEncoder().encode(text);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return toHex(new Uint8Array(digest));
}

async function hmacSha256Hex(keyText, messageText) {
  const keyData = new TextEncoder().encode(keyText);
  const cryptoKey = await crypto.subtle.importKey(
    "raw",
    keyData,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const sig = await crypto.subtle.sign("HMAC", cryptoKey, new TextEncoder().encode(messageText));
  return toHex(new Uint8Array(sig));
}

function randomNonce() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return toHex(bytes);
}

export function buildHeaders(token) {
  const headers = {
    "Content-Type": "application/json"
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

async function parseResponse(resp) {
  let body = null;
  try {
    body = await resp.json();
  } catch {
    body = null;
  }

  if (!resp.ok) {
    const code = body?.code || body?.errorCode || "http_error";
    const message = body?.message || body?.error || resp.statusText;
    throw new Error(`${code}: ${message}`);
  }
  return body;
}

export async function apiGet(path, token) {
  const resp = await fetch((await baseURL()) + path, {
    method: "GET",
    headers: buildHeaders(token)
  });
  return parseResponse(resp);
}

export async function apiPost(path, body, token) {
  const resp = await fetch((await baseURL()) + path, {
    method: "POST",
    headers: buildHeaders(token),
    body: JSON.stringify(body || {})
  });
  return parseResponse(resp);
}

export async function apiPostBridgeSigned(path, body, token) {
  const payload = JSON.stringify(body || {});
  const headers = buildHeaders(token);
  if (token) {
    const timestamp = Math.floor(Date.now() / 1000).toString();
    const nonce = randomNonce();
    const bodyHash = await sha256Hex(payload);
    const canonical = `${timestamp}\n${nonce}\n${bodyHash}`;
    const signature = await hmacSha256Hex(token, canonical);
    headers["X-Bridge-Timestamp"] = timestamp;
    headers["X-Bridge-Nonce"] = nonce;
    headers["X-Bridge-Signature"] = signature;
  }
  const resp = await fetch((await baseURL()) + path, {
    method: "POST",
    headers,
    body: payload
  });
  return parseResponse(resp);
}

export async function bridgeRotateToken(token) {
  return apiPost("/api/screenshot/bridge/token/rotate", { revoke_old: true }, token);
}
