import { dashboardFetch } from "@/api/http"

// API client for WebChat channel configuration.

interface WebChatTokenResponse {
  token: string
}

const BASE_URL = ""

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await dashboardFetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

// getWebChatToken reads the channel token the SPA passes to the same-origin
// /webchat/ws handshake as the "token.<value>" subprotocol.
export async function getWebChatToken(): Promise<WebChatTokenResponse> {
  return request<WebChatTokenResponse>("/api/webchat/token")
}

