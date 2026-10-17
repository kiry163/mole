import type { TFunction } from "i18next"
import { useTranslation } from "react-i18next"

import type { ApprovalOutcome, ApprovalRequest } from "@/api/approvals"
import { useApprovals } from "@/components/agent/approvals/use-approvals"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

const OUTCOME_KEYS: Record<ApprovalOutcome, string> = {
  allowed: "pages.approvals.outcome_allowed",
  always_allowed: "pages.approvals.outcome_always_allowed",
  denied: "pages.approvals.outcome_denied",
  expired: "pages.approvals.outcome_expired",
  cancelled: "pages.approvals.outcome_cancelled",
}

function originLabel(request: ApprovalRequest, t: TFunction): string {
  const channel = request.origin?.channel
  return channel
    ? t("pages.approvals.origin_chat") + " · " + channel
    : t("pages.approvals.origin_console")
}

// rememberableValue mirrors the server's rule in approvalEntryFor: only a
// non-blank string pinned argument can be remembered, so the card must not
// advertise a memory the always-allow API would refuse with a 400.
function rememberableValue(request: ApprovalRequest): string | undefined {
  if (!request.pin) return undefined
  const value = request.arguments?.[request.pin]
  if (typeof value !== "string" || value.trim() === "") return undefined
  return value
}

function ApprovalCard({ request }: { request: ApprovalRequest }) {
  const { t } = useTranslation()
  const { allow, alwaysAllow, deny, isDeciding } = useApprovals()
  const pinned = rememberableValue(request)

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div>
          <CardTitle className="text-base">{request.summary}</CardTitle>
          <p className="text-muted-foreground text-xs">
            {request.tool} · {request.category} · {originLabel(request, t)}
          </p>
        </div>
        <Badge variant="secondary">{t("pages.approvals.pending_badge")}</Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        {request.arguments && Object.keys(request.arguments).length > 0 ? (
          <pre className="bg-muted max-h-40 overflow-auto rounded p-2 text-xs">
            {JSON.stringify(request.arguments, null, 2)}
          </pre>
        ) : null}
        {pinned !== undefined ? (
          <p className="text-muted-foreground text-xs">
            {t("pages.approvals.remember_hint", {
              tool: request.tool,
              arg: request.pin,
              value: pinned,
            })}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button size="sm" disabled={isDeciding} onClick={() => allow(request.id)}>
            {t("pages.approvals.allow")}
          </Button>
          {pinned !== undefined ? (
            <Button size="sm" variant="secondary" disabled={isDeciding} onClick={() => alwaysAllow(request.id)}>
              {t("pages.approvals.always_allow")}
            </Button>
          ) : null}
          <Button size="sm" variant="destructive" disabled={isDeciding} onClick={() => deny(request.id)}>
            {t("pages.approvals.deny")}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function ApprovalsPage() {
  const { t } = useTranslation()
  const { pending, recent, isLoading } = useApprovals()

  return (
    <div className="h-full space-y-4 overflow-auto p-4">
      <div>
        <h1 className="text-lg font-semibold">{t("pages.approvals.title")}</h1>
        <p className="text-muted-foreground text-sm">{t("pages.approvals.subtitle")}</p>
      </div>

      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : pending.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("pages.approvals.empty")}</p>
      ) : (
        <div className="space-y-3">
          {pending.map((request) => (
            <ApprovalCard key={request.id} request={request} />
          ))}
        </div>
      )}

      {recent.length > 0 ? (
        <div className="space-y-2">
          <h2 className="text-sm font-medium">{t("pages.approvals.recent_title")}</h2>
          <ul className="space-y-1">
            {recent.map((record) => (
              <li key={record.id} className="text-muted-foreground flex items-center gap-2 text-xs">
                <Badge
                  variant={
                    record.outcome === "denied" || record.outcome === "expired" ? "destructive" : "outline"
                  }
                >
                  {t(OUTCOME_KEYS[record.outcome])}
                </Badge>
                <span>{record.summary}</span>
                {record.reason ? <span>· {record.reason}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
