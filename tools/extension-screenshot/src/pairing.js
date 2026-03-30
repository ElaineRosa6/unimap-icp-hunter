import { apiPost } from "./api.js";
import { saveSessionToken } from "./storage.js";

export async function requestPairToken(clientId, pairCode) {
  const body = {
    client_id: clientId,
    pair_code: pairCode
  };
  return apiPost("/api/screenshot/bridge/pair", body, "");
}

export async function pairAndStore(clientId, pairCode) {
  const resp = await requestPairToken(clientId, pairCode);
  const token = resp?.token || "";
  const expiresIn = Number(resp?.expires_in || 600);
  if (!token) {
    throw new Error("pairing_failed: empty token");
  }
  await saveSessionToken(token, expiresIn);
  return { token, expiresIn };
}
