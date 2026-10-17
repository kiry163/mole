import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { decideApproval, listApprovals, type ApprovalDecision } from "@/api/approvals"

export const APPROVALS_KEY = ["approvals"] as const
const APPROVALS_POLL_INTERVAL_MS = 1000

export function useApprovals() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: APPROVALS_KEY,
    queryFn: listApprovals,
    refetchInterval: APPROVALS_POLL_INTERVAL_MS,
  })

  const decide = useMutation({
    mutationFn: ({ id, decision }: { id: string; decision: ApprovalDecision }) => decideApproval(id, decision),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: APPROVALS_KEY })
      if (variables.decision === "deny") {
        toast.success(t("pages.approvals.denied"))
      } else if (variables.decision === "always_allow") {
        toast.success(t("pages.approvals.always_allowed"))
      } else {
        toast.success(t("pages.approvals.allowed"))
      }
    },
    onError: (error: unknown) => {
      toast.error(error instanceof Error ? error.message : t("pages.approvals.decide_error"))
    },
  })

  return {
    pending: query.data?.pending ?? [],
    recent: query.data?.recent ?? [],
    isLoading: query.isLoading,
    allow: (id: string) => decide.mutate({ id, decision: "allow" }),
    alwaysAllow: (id: string) => decide.mutate({ id, decision: "always_allow" }),
    deny: (id: string) => decide.mutate({ id, decision: "deny" }),
    isDeciding: decide.isPending,
  }
}
