import { expect, vi } from "vitest";
import { request } from "./client.js";
export { request } from "./client.js";
async function challenge(verifier) {
  return btoa(
    String.fromCharCode(
      ...new Uint8Array(
        await crypto.subtle.digest(
          "SHA-256",
          new TextEncoder().encode(verifier),
        ),
      ),
    ),
  )
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

export const origin = "http://localhost:8787";
function cookies(response) {
  return response.headers
    .getSetCookie()
    .map((cookie) => cookie.split(";")[0])
    .join("; ");
}

export async function startLogin(
  scopes = "tasks:read tasks:write offline_access",
  name = "Test client",
  redirectURI = "http://localhost:4242/callback",
) {
  const verifier = crypto.randomUUID() + crypto.randomUUID();
  const registration = await request("/oauth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      client_name: name,
      redirect_uris: [redirectURI],
      token_endpoint_auth_method: "none",
      grant_types: ["authorization_code", "refresh_token"],
      response_types: ["code"],
    }),
  });
  expect(registration.status).toBe(201);
  const client = await registration.json();
  const params = new URLSearchParams({
    response_type: "code",
    client_id: client.client_id,
    redirect_uri: redirectURI,
    scope: scopes,
    state: "client-state",
    code_challenge: await challenge(verifier),
    code_challenge_method: "S256",
    resource: origin,
  });
  const consent = await request("/authorize?" + params);
  expect(consent.status).toBe(200);
  const html = await consent.text();
  const handle = /name="handle" value="([^"]+)"/.exec(html)[1];
  return {
    verifier,
    client,
    params,
    consent,
    html,
    handle,
    cookie: cookies(consent),
  };
}

export async function signIn(scopes, githubID = 12345) {
  const login = await startLogin(scopes);
  const redirect = await request("/authorize", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Origin: origin,
      Cookie: login.cookie,
    },
    body: new URLSearchParams({ handle: login.handle, decision: "approve" }),
  });
  expect(redirect.status).toBe(302);
  const location = new URL(redirect.headers.get("Location"));
  const state = location.searchParams.get("state");
  const spy = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url === "https://github.com/login/oauth/access_token")
        return Response.json({ access_token: "upstream-test-token" });
      if (url === "https://api.github.com/user")
        return Response.json({ id: githubID });
      throw new Error("Unexpected external request: " + url);
    });
  let callback;
  try {
    callback = await request(
      "/callback?" + new URLSearchParams({ state, code: "github-test-code" }),
      {
        headers: { Cookie: cookies(redirect) },
      },
    );
  } finally {
    spy.mockRestore();
  }
  if (callback.status !== 302)
    return { callback, login, token: "", refreshToken: "" };
  const target = new URL(callback.headers.get("Location"));
  expect(target.searchParams.get("state")).toBe("client-state");
  const tokenResponse = await request("/oauth/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      code: target.searchParams.get("code"),
      client_id: login.client.client_id,
      redirect_uri: login.params.get("redirect_uri"),
      code_verifier: login.verifier,
      resource: origin,
    }),
  });
  expect(tokenResponse.status).toBe(200);
  const result = await tokenResponse.json();

  return {
    callback,
    login,
    token: result.access_token,
    refreshToken: result.refresh_token,
  };
}
