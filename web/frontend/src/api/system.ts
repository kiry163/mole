import { dashboardFetch } from "@/api/http"

export interface SystemVersionInfo {
  version: string
  git_commit?: string
  build_time?: string
  go_version: string
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await dashboardFetch(path, options)
  if (!res.ok) {
    let message = `API error: ${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as {
        error?: string
        errors?: string[]
      }
      if (Array.isArray(body.errors) && body.errors.length > 0) {
        message = body.errors.join("; ")
      } else if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
    } catch {
      // Keep fallback error message when response body is not JSON.
    }
    throw new Error(message)
  }
  return res.json() as Promise<T>
}

export async function getSystemVersionInfo(): Promise<SystemVersionInfo> {
  return request<SystemVersionInfo>("/api/system/version")
}
