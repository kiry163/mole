import { dashboardFetch } from "@/api/http"

// API client for the gateway this console runs inside. The gateway and the
// console share one process and one listener, so the console only reads the
// state: there is nothing here to start, stop or restart.

// GatewayStatusResponse is the part of GET /api/gateway/status the console reads.
// The endpoint's always-false start flags and the identity fields it also reports
// (pid, version, address, channels) are not rendered by the console.
export interface GatewayStatusResponse {
  status: "running" | "unknown"
}

// GatewayLogsResponse is the payload of GET /api/gateway/logs: a tail window of
// the gateway log, oldest line first, plus the line count of the whole file.
export interface GatewayLogsResponse {
  logs: string[]
  log_total: number
}

const BASE_URL = ""

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await dashboardFetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function getGatewayStatus(): Promise<GatewayStatusResponse> {
  return request<GatewayStatusResponse>("/api/gateway/status")
}

// getGatewayLogs reads one tail window of the gateway log: limit is the window
// size in lines and offset counts back from the newest line, so each call
// returns the last `limit` lines rather than a resumable cursor.
export async function getGatewayLogs(options?: {
  offset?: number
  limit?: number
}): Promise<GatewayLogsResponse> {
  const params = new URLSearchParams()
  if (options?.offset !== undefined) {
    params.set("offset", options.offset.toString())
  }
  if (options?.limit !== undefined) {
    params.set("limit", options.limit.toString())
  }
  const queryString = params.toString() ? `?${params.toString()}` : ""
  return request<GatewayLogsResponse>(`/api/gateway/logs${queryString}`)
}
