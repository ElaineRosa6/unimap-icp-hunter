function baseURL() {
  return "http://127.0.0.1:8448";
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
  const resp = await fetch(baseURL() + path, {
    method: "GET",
    headers: buildHeaders(token)
  });
  return parseResponse(resp);
}

export async function apiPost(path, body, token) {
  const resp = await fetch(baseURL() + path, {
    method: "POST",
    headers: buildHeaders(token),
    body: JSON.stringify(body || {})
  });
  return parseResponse(resp);
}
