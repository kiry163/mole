import { isAuthPathname } from "@/lib/auth-path"

function isOnAuthPath(): boolean {
  if (typeof globalThis.location === "undefined") {
    return false
  }
  if (isAuthPathname(globalThis.location.pathname || "/")) {
    return true
  }
  try {
    return isAuthPathname(new URL(globalThis.location.href).pathname || "/")
  } catch {
    return false
  }
}

/**
 * Same-origin fetch that sends cookies; redirects to the login page on 401 JSON responses.
 * Skips redirect while already on an auth page (login or setup) to avoid reload loops.
 */
export async function dashboardFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const res = await fetch(input, {
    credentials: "same-origin",
    ...init,
  })
  if (res.status === 401) {
    const ct = res.headers.get("content-type") || ""
    if (
      ct.includes("application/json") &&
      typeof globalThis.location !== "undefined" &&
      !isOnAuthPath()
    ) {
      globalThis.location.assign("/login")
    }
  }
  return res
}
