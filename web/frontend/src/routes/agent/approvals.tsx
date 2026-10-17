import { createFileRoute } from "@tanstack/react-router"

import { ApprovalsPage } from "@/components/agent/approvals/approvals-page"

export const Route = createFileRoute("/agent/approvals")({
  component: AgentApprovalsRoute,
})

function AgentApprovalsRoute() {
  return <ApprovalsPage />
}
