import { atom, getDefaultStore } from "jotai"

import { getGatewayStatus } from "@/api/gateway"

// The console is served by the gateway process it reports, so the only states
// that exist are "running" and "unknown" (no runtime attached yet, or the
// status poll has not succeeded).
export type GatewayState = "running" | "unknown"

export interface GatewayStoreState {
  status: GatewayState
}

const DEFAULT_GATEWAY_STATE: GatewayStoreState = {
  status: "unknown",
}

const GATEWAY_POLL_INTERVAL_MS = 2000

interface RefreshGatewayStateOptions {
  force?: boolean
}

// Global atom for gateway state
export const gatewayAtom = atom<GatewayStoreState>(DEFAULT_GATEWAY_STATE)

let gatewayPollingSubscribers = 0
let gatewayPollingTimer: ReturnType<typeof setTimeout> | undefined
let gatewayPollingRequest: Promise<void> | null = null

function setGatewayStatus(status: GatewayState) {
  getDefaultStore().set(gatewayAtom, (prev) =>
    prev.status === status ? prev : { status },
  )
}

function scheduleGatewayPoll() {
  if (gatewayPollingSubscribers === 0) {
    return
  }

  clearTimeout(gatewayPollingTimer)
  gatewayPollingTimer = setTimeout(() => {
    gatewayPollingTimer = undefined
    void refreshGatewayState()
  }, GATEWAY_POLL_INTERVAL_MS)
}

export async function refreshGatewayState(
  options: RefreshGatewayStateOptions = {},
) {
  if (gatewayPollingRequest) {
    await gatewayPollingRequest
    if (options.force) {
      await refreshGatewayState()
    }
    return
  }

  gatewayPollingRequest = (async () => {
    try {
      const status = await getGatewayStatus()
      setGatewayStatus(status.status)
    } catch {
      // Preserve the last known state when a poll fails.
    } finally {
      gatewayPollingRequest = null
      scheduleGatewayPoll()
    }
  })()

  try {
    await gatewayPollingRequest
  } finally {
    if (gatewayPollingSubscribers === 0) {
      clearTimeout(gatewayPollingTimer)
      gatewayPollingTimer = undefined
    }
  }
}

export function subscribeGatewayPolling() {
  gatewayPollingSubscribers += 1
  if (gatewayPollingSubscribers === 1) {
    void refreshGatewayState()
  }

  return () => {
    gatewayPollingSubscribers = Math.max(0, gatewayPollingSubscribers - 1)
    if (gatewayPollingSubscribers === 0) {
      clearTimeout(gatewayPollingTimer)
      gatewayPollingTimer = undefined
    }
  }
}
