/** True for any page that is part of the auth flow (login or setup). */
export function isAuthPathname(pathname: string): boolean {
  const normalized = pathname.replace(/\/+$/, "") || "/"
  return normalized === "/login" || normalized === "/setup"
}
