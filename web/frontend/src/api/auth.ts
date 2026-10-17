/**
 * Dashboard session auth API.
 * Uses plain fetch (not dashboardFetch) to avoid redirect loops on auth pages.
 */
export type LoginResult =
  | { ok: true }
  | { ok: false; status: number; error: string }

export async function postDashboardLogin(
  password: string,
): Promise<LoginResult> {
  const res = await fetch("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ password: password.trim() }),
  })
  if (res.ok) return { ok: true }

  return {
    ok: false,
    status: res.status,
    error: await readAuthError(res),
  }
}

export type AuthStatus = {
  authenticated: boolean
  /** true when dashboard.token is configured */
  initialized: boolean
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const res = await fetch("/api/auth/status", {
    method: "GET",
    credentials: "same-origin",
  })
  if (!res.ok) {
    throw new Error(`status ${res.status}`)
  }
  return (await res.json()) as AuthStatus
}

export async function postDashboardLogout(): Promise<boolean> {
  const res = await fetch("/api/auth/logout", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: "{}",
  })
  return res.ok
}

export type SetupResult = { ok: true } | { ok: false; error: string }

export async function postDashboardSetup(
  password: string,
  confirm: string,
): Promise<SetupResult> {
  const res = await fetch("/api/auth/setup", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({
      password: password.trim(),
      confirm: confirm.trim(),
    }),
  })
  if (res.ok) return { ok: true }
  return { ok: false, error: await readAuthError(res) }
}

async function readAuthError(res: Response): Promise<string> {
  let msg = `Request failed with status ${res.status}`
  try {
    const j = (await res.json()) as { error?: string }
    if (j.error) msg = j.error
  } catch {
    /* ignore */
  }
  return msg
}
