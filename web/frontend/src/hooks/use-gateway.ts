import { useAtomValue } from "jotai"
import { useCallback, useEffect } from "react"

import {
  gatewayAtom,
  refreshGatewayState,
  subscribeGatewayPolling,
} from "@/store"

// useGateway exposes the read-only gateway state the console renders. The
// console runs inside the gateway process, so it can only report that state and
// ask for a fresh poll: nothing here starts, stops or restarts anything.
export function useGateway() {
  const gateway = useAtomValue(gatewayAtom)

  useEffect(() => {
    return subscribeGatewayPolling()
  }, [])

  const refresh = useCallback(() => refreshGatewayState({ force: true }), [])

  return { state: gateway.status, refresh }
}
