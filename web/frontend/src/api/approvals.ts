import { dashboardFetch } from "@/api/http"

export type ApprovalOutcome = "allowed" | "always_allowed" | "denied" | "expired" | "cancelled"

export interface ApprovalOrigin {
  channel?: string
  chat_id?: string
  sender_id?: string
  session_key?: string
  turn_id?: string
  iteration?: number
}

export interface ApprovalRequest {
  id: string
  tool: string
  category: string
  summary: string
  pin?: string
  arguments?: Record<string, unknown>
  origin: ApprovalOrigin
  created_at: string
  deadline?: string
}

export interface ApprovalRecord {
  id: string
  tool: string
  summary: string
  outcome: ApprovalOutcome
  reason?: string
  decided_by?: string
  created_at: string
  decided_at: string
}

export interface ApprovalQueue {
  pending: ApprovalRequest[]
  recent: ApprovalRecord[]
}

export type ApprovalDecision = "allow" | "deny" | "always_allow"

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await dashboardFetch(path, options)
  if (!res.ok) {
    throw new Error((await res.text()).trim() || `Request failed: ${res.status}`)
  }
  return res.json() as Promise<T>
}

export function listApprovals(): Promise<ApprovalQueue> {
  return request<ApprovalQueue>("/api/approvals")
}

export function decideApproval(id: string, decision: ApprovalDecision): Promise<{ status: string }> {
  return request<{ status: string }>(`/api/approvals/${encodeURIComponent(id)}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ decision }),
  })
}
