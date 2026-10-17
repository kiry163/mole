import { dashboardFetch } from "@/api/http"

export async function listBuiltinHooks(): Promise<string[]> {
  const res = await dashboardFetch("/api/hooks/builtins")
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  const body = (await res.json()) as { builtins?: string[] }
  return body.builtins ?? []
}
