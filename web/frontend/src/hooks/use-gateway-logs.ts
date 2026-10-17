import { useEffect, useRef, useState } from "react"

import { getGatewayLogs } from "@/api/gateway"

// LOG_TAIL_LIMIT is the window size the log page shows: the newest lines of the
// gateway log. /api/gateway/logs returns a tail window rather than a cursor, so
// every poll replaces the whole window instead of appending to it.
const LOG_TAIL_LIMIT = 200
const LOG_POLL_INTERVAL_MS = 1000

export function useGatewayLogs() {
  const [logs, setLogs] = useState<string[]>([])
  const logTotalRef = useRef(-1)

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined

    const pollLogs = async () => {
      try {
        const data = await getGatewayLogs({ limit: LOG_TAIL_LIMIT })
        // The window is rebuilt from the tail on every poll, so it only has to
        // reach the UI when the log grew, rotated or was truncated.
        if (!cancelled && data.log_total !== logTotalRef.current) {
          logTotalRef.current = data.log_total
          setLogs(data.logs)
        }
      } catch {
        // Ignore simple fetch errors during polling.
      } finally {
        if (!cancelled) {
          timer = setTimeout(pollLogs, LOG_POLL_INTERVAL_MS)
        }
      }
    }

    void pollLogs()

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [])

  return { logs }
}
