import { IconCode, IconDeviceFloppy, IconTag } from "@tabler/icons-react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { postDashboardSetup } from "@/api/auth"
import { patchAppConfig, resetAppConfig } from "@/api/channels"
import { dashboardFetch } from "@/api/http"
import { getSystemVersionInfo } from "@/api/system"
import { ConfigChangeNotice } from "@/components/config-change-notice"
import { HooksSection } from "@/components/config/hooks-section"
import {
  AgentDefaultsSection,
  DashboardAccessSection,
  ExecSection,
  MCPSection,
  RuntimeSection,
} from "@/components/config/config-sections"
import {
  type CoreConfigForm,
  type DashboardPasswordForm,
  EMPTY_DASHBOARD_PASSWORD_FORM,
  EMPTY_FORM,
  type HookProcessForm,
  type MCPServerForm,
  type TurnProfileForm,
  buildFormFromConfig,
  parseIntField,
  parseCommaOrNewlineList,
  parseJSONObjectField,
  parseMultilineList,
} from "@/components/config/form-model"
import { PageHeader } from "@/components/page-header"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { refreshGatewayState } from "@/store/gateway"

function buildStringMapMergePatch(
  next: Record<string, string>,
  previous: Record<string, string>,
): Record<string, string | null> {
  const patch: Record<string, string | null> = { ...next }

  for (const key of Object.keys(previous)) {
    if (!(key in next)) {
      patch[key] = null
    }
  }

  return patch
}

function buildTurnProfilePatch(
  profile: TurnProfileForm,
): Record<string, unknown> {
  const result: Record<string, unknown> = {
    enabled: profile.enabled,
    history: { mode: profile.historyMode },
    system_prompt: { mode: profile.systemPromptMode },
    skills: { mode: profile.skillsMode },
    tools: { mode: profile.toolsMode },
  }

  if (profile.skillsMode === "custom") {
    result.skills = {
      mode: "custom",
      allow: parseMultilineList(profile.skillsAllowText),
    }
  }
  if (profile.toolsMode === "custom") {
    result.tools = {
      mode: "custom",
      allow: parseMultilineList(profile.toolsAllowText),
    }
  }

  return result
}

export function ConfigPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<CoreConfigForm>(EMPTY_FORM)
  const [baseline, setBaseline] = useState<CoreConfigForm>(EMPTY_FORM)
  const [passwordForm, setPasswordForm] = useState<DashboardPasswordForm>(
    EMPTY_DASHBOARD_PASSWORD_FORM,
  )
  const [saving, setSaving] = useState(false)
  const [showFactoryResetDialog, setShowFactoryResetDialog] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: ["config"],
    queryFn: async () => {
      const controller = new AbortController()
      const timer = setTimeout(() => controller.abort(), 5000)
      try {
        const res = await dashboardFetch("/api/config", {
          signal: controller.signal,
        })
        if (!res.ok) {
          throw new Error("Failed to load config")
        }
        return res.json()
      } finally {
        clearTimeout(timer)
      }
    },
  })

  const { data: versionInfo } = useQuery({
    queryKey: ["system", "version"],
    queryFn: getSystemVersionInfo,
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (!data) return
    const parsed = buildFormFromConfig(data)
    setForm(parsed)
    setBaseline(parsed)
  }, [data])

  const configDirty = JSON.stringify(form) !== JSON.stringify(baseline)
  const passwordDirty =
    passwordForm.password.trim() !== "" || passwordForm.confirm.trim() !== ""
  const isDirty = configDirty || passwordDirty

  const updateField = <K extends keyof CoreConfigForm>(
    key: K,
    value: CoreConfigForm[K],
  ) => {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const updatePasswordField = <K extends keyof DashboardPasswordForm>(
    key: K,
    value: DashboardPasswordForm[K],
  ) => {
    setPasswordForm((prev) => ({ ...prev, [key]: value }))
  }

  const handleMCPServerAdd = () => {
    const nextIndex = form.mcpServers.length + 1
    const server: MCPServerForm = {
      id: `mcp-${Date.now()}-${nextIndex}`,
      name: "",
      enabled: true,
      deferredOverride: null,
      type: "stdio",
      url: "",
      command: "",
      argsText: "",
      envText: "{}",
      envFile: "",
      headersText: "{}",
    }
    updateField("mcpServers", [...form.mcpServers, server])
  }

  const handleMCPServerRemove = (id: string) => {
    updateField(
      "mcpServers",
      form.mcpServers.filter((server) => server.id !== id),
    )
  }

  const handleMCPServerFieldChange = <K extends keyof MCPServerForm>(
    id: string,
    key: K,
    value: MCPServerForm[K],
  ) => {
    updateField(
      "mcpServers",
      form.mcpServers.map((server) =>
        server.id === id ? { ...server, [key]: value } : server,
      ),
    )
  }

  const handleHookProcessAdd = () => {
    const nextIndex = form.hookProcesses.length + 1
    updateField("hookProcesses", [
      ...form.hookProcesses,
      {
        id: `hook-process-${Date.now()}-${nextIndex}`,
        name: "",
        enabled: true,
        priority: "0",
        command: "",
        argsText: "",
        dir: "",
        envText: "{}",
        observeText: "",
        interceptText: "",
      },
    ])
  }

  const handleHookProcessRemove = (id: string) => {
    updateField(
      "hookProcesses",
      form.hookProcesses.filter((process) => process.id !== id),
    )
  }

  const handleHookProcessFieldChange = <K extends keyof HookProcessForm>(
    id: string,
    key: K,
    value: HookProcessForm[K],
  ) => {
    updateField(
      "hookProcesses",
      form.hookProcesses.map((process) =>
        process.id === id ? { ...process, [key]: value } : process,
      ),
    )
  }

  const handleHookBuiltinAdd = (name: string) => {
    if (name === "" || form.hookBuiltins.some((builtin) => builtin.name === name)) {
      return
    }
    updateField("hookBuiltins", [
      ...form.hookBuiltins,
      { id: `hook-builtin-${Date.now()}`, name, enabled: true, priority: "0" },
    ])
  }

  const handleHookBuiltinRemove = (id: string) => {
    updateField(
      "hookBuiltins",
      form.hookBuiltins.filter((builtin) => builtin.id !== id),
    )
  }

  const handleHookBuiltinToggle = (id: string, enabled: boolean) => {
    updateField(
      "hookBuiltins",
      form.hookBuiltins.map((builtin) => (builtin.id === id ? { ...builtin, enabled } : builtin)),
    )
  }

  const handleTurnProfileFieldChange = <K extends keyof TurnProfileForm>(
    key: K,
    value: TurnProfileForm[K],
  ) => {
    updateField("turnProfile", { ...form.turnProfile, [key]: value })
  }

  const handleReset = () => {
    setForm(baseline)
    setPasswordForm(EMPTY_DASHBOARD_PASSWORD_FORM)
    toast.info(t("pages.config.reset_success"))
  }

  const handleFactoryReset = async () => {
    try {
      await resetAppConfig()
      const fresh = await dashboardFetch("/api/config").then((r) => r.json())
      const parsed = buildFormFromConfig(fresh)
      setForm(parsed)
      setBaseline(parsed)
      await queryClient.invalidateQueries({ queryKey: ["config"] })
      await refreshGatewayState()
      toast.success(t("pages.config.factory_reset_success"))
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t("pages.config.factory_reset_error"),
      )
    } finally {
      setShowFactoryResetDialog(false)
    }
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      const password = passwordForm.password.trim()
      const confirm = passwordForm.confirm.trim()
      if (passwordDirty) {
        if (!password) {
          throw new Error(t("pages.config.dashboard_password_required"))
        }
        if (password !== confirm) {
          throw new Error(t("pages.config.dashboard_password_mismatch"))
        }
        if (Array.from(password).length < 8) {
          throw new Error(t("pages.config.dashboard_password_min_length"))
        }
      }

      if (configDirty) {
        const workspace = form.workspace.trim()
        const dmScope = form.dmScope.trim()

        if (!workspace) {
          throw new Error("Workspace path is required.")
        }
        if (!dmScope) {
          throw new Error("Session scope is required.")
        }

        if (
          form.mcpEnabled &&
          form.mcpDiscoveryEnabled &&
          !form.mcpDiscoveryUseBM25 &&
          !form.mcpDiscoveryUseRegex
        ) {
          throw new Error(
            "MCP discovery requires at least one search method (BM25 or regex).",
          )
        }

        const maxTokens = parseIntField(form.maxTokens, "Max tokens", {
          min: 1,
        })
        const contextWindow = form.contextWindow.trim()
          ? parseIntField(form.contextWindow, "Context window", { min: 1 })
          : undefined
        const maxToolIterations = parseIntField(
          form.maxToolIterations,
          "Max tool iterations",
          { min: 1 },
        )
        const toolFeedbackMaxArgsLength = parseIntField(
          form.toolFeedbackMaxArgsLength,
          "Tool feedback max args length",
          { min: 0 },
        )
        const summarizeMessageThreshold = parseIntField(
          form.summarizeMessageThreshold,
          "Summarize message threshold",
          { min: 1 },
        )
        const summarizeTokenPercent = parseIntField(
          form.summarizeTokenPercent,
          "Summarize token percent",
          { min: 1, max: 100 },
        )
        const turnProfile = buildTurnProfilePatch(form.turnProfile)
        const mcpDiscoveryValidationEnabled =
          form.mcpEnabled && form.mcpDiscoveryEnabled
        const mcpDiscoveryPatch: Record<string, unknown> = {
          enabled: form.mcpDiscoveryEnabled,
          use_bm25: form.mcpDiscoveryUseBM25,
          use_regex: form.mcpDiscoveryUseRegex,
        }

        if (mcpDiscoveryValidationEnabled) {
          mcpDiscoveryPatch.ttl = parseIntField(
            form.mcpDiscoveryTTL,
            "MCP discovery ttl",
            {
              min: 1,
            },
          )
          mcpDiscoveryPatch.max_search_results = parseIntField(
            form.mcpDiscoveryMaxSearchResults,
            "MCP discovery max search results",
            { min: 1 },
          )
        }
        const execConfigPatch: Record<string, unknown> = {
          enabled: form.execEnabled,
        }

        let mcpServersPatch: Record<string, Record<string, unknown> | null> = {}
        if (form.mcpEnabled) {
          const baselineServerNames = new Set(
            baseline.mcpServers
              .map((server) => server.name.trim())
              .filter((name) => name !== ""),
          )

          const normalizedServers = form.mcpServers
            .map((server) => ({
              ...server,
              name: server.name.trim(),
              url: server.url.trim(),
              command: server.command.trim(),
              envFile: server.envFile.trim(),
            }))
            .filter((server) => server.name !== "")

          const serverNameCounts = new Map<string, number>()
          for (const server of normalizedServers) {
            serverNameCounts.set(
              server.name,
              (serverNameCounts.get(server.name) ?? 0) + 1,
            )
          }

          const duplicateNames = Array.from(serverNameCounts.entries())
            .filter(([, count]) => count > 1)
            .map(([name]) => name)
            .sort((a, b) => a.localeCompare(b))

          if (duplicateNames.length > 0) {
            throw new Error(
              `MCP server names must be unique. Duplicates: ${duplicateNames.join(", ")}.`,
            )
          }

          const currentServerNames = new Set(
            normalizedServers.map((server) => server.name),
          )

          const removedServerEntries = Array.from(baselineServerNames)
            .filter((name) => !currentServerNames.has(name))
            .map((name) => [name, null] as const)

          const baselineServersByName = new Map(
            baseline.mcpServers
              .map((server) => ({
                ...server,
                name: server.name.trim(),
              }))
              .filter((server) => server.name !== "")
              .map((server) => [server.name, server] as const),
          )

          const upsertServerEntries = normalizedServers.map((server) => {
            const deferredPatch = { deferred: server.deferredOverride }
            const baselineServer = baselineServersByName.get(server.name)
            const shouldValidateServer = server.enabled

            if (server.type !== "stdio") {
              if (shouldValidateServer && server.url === "") {
                throw new Error(`MCP server ${server.name} requires a URL.`)
              }

              if (shouldValidateServer) {
                try {
                  const parsedURL = new URL(server.url)
                  if (
                    parsedURL.protocol !== "http:" &&
                    parsedURL.protocol !== "https:"
                  ) {
                    throw new Error("invalid protocol")
                  }
                } catch {
                  throw new Error(
                    `MCP server ${server.name} requires a valid HTTP(S) URL.`,
                  )
                }
              }

              const baselineHeaders = baselineServer
                ? parseJSONObjectField(
                    baselineServer.headersText,
                    `Saved MCP server ${server.name} headers`,
                  )
                : {}

              return [
                server.name,
                {
                  ...deferredPatch,
                  enabled: server.enabled,
                  type: server.type,
                  url: server.url,
                  headers: buildStringMapMergePatch(
                    shouldValidateServer
                      ? parseJSONObjectField(
                          server.headersText,
                          `MCP server ${server.name} headers`,
                        )
                      : baselineHeaders,
                    baselineHeaders,
                  ),
                  command: null,
                  args: null,
                  env: null,
                  env_file: null,
                },
              ] as const
            }

            if (shouldValidateServer && server.command === "") {
              throw new Error(`MCP server ${server.name} requires a command.`)
            }

            const baselineEnv = baselineServer
              ? parseJSONObjectField(
                  baselineServer.envText,
                  `Saved MCP server ${server.name} env`,
                )
              : {}

            return [
              server.name,
              {
                ...deferredPatch,
                enabled: server.enabled,
                type: "stdio",
                command: server.command,
                args: parseMultilineList(server.argsText),
                env: buildStringMapMergePatch(
                  shouldValidateServer
                    ? parseJSONObjectField(
                        server.envText,
                        `MCP server ${server.name} env`,
                      )
                    : baselineEnv,
                  baselineEnv,
                ),
                env_file: server.envFile === "" ? null : server.envFile,
                url: null,
                headers: null,
              },
            ] as const
          })

          mcpServersPatch = Object.fromEntries([
            ...upsertServerEntries,
            ...removedServerEntries,
          ])
        }

        if (form.execEnabled) {
          execConfigPatch.allow_remote = form.allowRemote
          execConfigPatch.enable_deny_patterns = form.enableDenyPatterns
          execConfigPatch.custom_allow_patterns = parseMultilineList(
            form.customAllowPatternsText,
          )
          execConfigPatch.timeout_seconds = parseIntField(
            form.execTimeoutSeconds,
            "Exec timeout",
            { min: 0 },
          )

          if (form.enableDenyPatterns) {
            execConfigPatch.custom_deny_patterns = parseMultilineList(
              form.customDenyPatternsText,
            )
          }
        }

        // Hooks are a map of named entries: upsert what the form holds and null
        // out what it dropped (PATCH is a recursive merge).
        const hookProcessEntries: Record<string, Record<string, unknown> | null> = {}
        const baselineProcessNames = new Set(
          baseline.hookProcesses.map((process) => process.name.trim()).filter((name) => name !== ""),
        )
        for (const process of form.hookProcesses) {
          const name = process.name.trim()
          if (name === "") {
            continue
          }
          const command = [
            process.command.trim(),
            ...process.argsText.trim().split(/\s+/).filter((part) => part !== ""),
          ].filter((part) => part !== "")
          hookProcessEntries[name] = {
            enabled: process.enabled,
            priority: parseIntField(
              process.priority,
              `${t("pages.config.hooks_processes")} ${name}`,
              { min: 0 },
            ),
            transport: "stdio",
            command,
            dir: process.dir.trim(),
            env: parseJSONObjectField(
              process.envText,
              `${t("pages.config.hooks_process_env")} ${name}`,
            ),
            observe: parseCommaOrNewlineList(process.observeText),
            intercept: parseCommaOrNewlineList(process.interceptText),
          }
          baselineProcessNames.delete(name)
        }
        for (const name of baselineProcessNames) {
          hookProcessEntries[name] = null
        }

        const hookBuiltinEntries: Record<string, Record<string, unknown> | null> = {}
        const baselineBuiltinNames = new Set(
          baseline.hookBuiltins.map((builtin) => builtin.name.trim()).filter((name) => name !== ""),
        )
        for (const builtin of form.hookBuiltins) {
          const name = builtin.name.trim()
          if (name === "") {
            continue
          }
          hookBuiltinEntries[name] = {
            enabled: builtin.enabled,
            priority: parseIntField(
              builtin.priority,
              `${t("pages.config.hooks_builtins")} ${name}`,
              { min: 0 },
            ),
          }
          baselineBuiltinNames.delete(name)
        }
        for (const name of baselineBuiltinNames) {
          hookBuiltinEntries[name] = null
        }

        await patchAppConfig({
          agents: {
            defaults: {
              workspace,
              restrict_to_workspace: form.restrictToWorkspace,
              split_on_marker: form.splitOnMarker,
              tool_feedback: {
                enabled: form.toolFeedbackEnabled,
                max_args_length: toolFeedbackMaxArgsLength,
                separate_messages: form.toolFeedbackSeparateMessages,
              },
              max_tokens: maxTokens,
              context_window: contextWindow,
              max_tool_iterations: maxToolIterations,
              summarize_message_threshold: summarizeMessageThreshold,
              summarize_token_percent: summarizeTokenPercent,
              turn_profile: turnProfile,
            },
          },
          session: {
            dm_scope: dmScope,
          },
          hooks: {
            enabled: form.hooksEnabled,
            defaults: {
              observer_timeout_ms: parseIntField(
                form.hooksObserverTimeoutMs,
                t("pages.config.hooks_observer_timeout"),
                { min: 1 },
              ),
              interceptor_timeout_ms: parseIntField(
                form.hooksInterceptorTimeoutMs,
                t("pages.config.hooks_interceptor_timeout"),
                { min: 1 },
              ),
              approval_timeout_ms: parseIntField(
                form.hooksApprovalTimeoutMs,
                t("pages.config.hooks_approval_timeout"),
                { min: 1 },
              ),
            },
            processes: hookProcessEntries,
            builtins: hookBuiltinEntries,
          },
          tools: {
            exec: execConfigPatch,
            mcp: {
              enabled: form.mcpEnabled,
              discovery: mcpDiscoveryPatch,
              servers: mcpServersPatch,
            },
          },
        })

        setBaseline(form)
        queryClient.invalidateQueries({ queryKey: ["config"] })
      }

      if (passwordDirty) {
        const result = await postDashboardSetup(password, confirm)
        if (!result.ok) {
          throw new Error(result.error)
        }

        setPasswordForm(EMPTY_DASHBOARD_PASSWORD_FORM)
      }

      await refreshGatewayState({ force: true })
      toast.success(t("pages.config.save_success"))
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t("pages.config.save_error"),
      )
    } finally {
      setSaving(false)
    }
  }

  const factoryResetButton = (
    <AlertDialog
      open={showFactoryResetDialog}
      onOpenChange={setShowFactoryResetDialog}
    >
      <AlertDialogTrigger asChild>
        <Button variant="destructive" disabled={saving}>
          {t("pages.config.factory_reset")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("pages.config.factory_reset_confirm_title")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("pages.config.factory_reset_confirm_desc")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={handleFactoryReset}>
            {t("pages.config.factory_reset_confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )

  const actionButtons = (
    <div className="flex justify-end gap-2">
      {factoryResetButton}
      <Button
        variant="outline"
        onClick={handleReset}
        disabled={!isDirty || saving}
      >
        {t("common.reset")}
      </Button>
      <Button onClick={handleSave} disabled={!isDirty || saving}>
        <IconDeviceFloppy className="size-4" />
        {saving ? t("common.saving") : t("common.save")}
      </Button>
    </div>
  )

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={t("navigation.config")}
        titleExtra={
          versionInfo && (
            <Badge
              variant="secondary"
              className="gap-1 font-mono text-[11px] font-normal opacity-80"
            >
              <IconTag className="size-3 opacity-70" />
              {versionInfo.version}
            </Badge>
          )
        }
        children={
          <Button variant="outline" asChild>
            <Link to="/config/raw">
              <IconCode className="size-4" />
              {t("pages.config.open_raw")}
            </Link>
          </Button>
        }
      />
      <div className="flex-1 overflow-auto p-3 lg:p-6">
        <div className="mx-auto w-full max-w-[1000px] space-y-6">
          {isLoading ? (
            <div className="text-muted-foreground py-6 text-sm">
              {t("labels.loading")}
            </div>
          ) : error ? (
            <div className="space-y-4">
              <div className="text-destructive py-6 text-sm">
                {t("pages.config.load_error")}
              </div>
              <div className="flex justify-end">{factoryResetButton}</div>
            </div>
          ) : (
            <div className="space-y-6">
              <DashboardAccessSection
                passwordForm={passwordForm}
                onPasswordFieldChange={updatePasswordField}
                disabled={saving}
              />

              <AgentDefaultsSection
                form={form}
                onFieldChange={updateField}
                onTurnProfileFieldChange={handleTurnProfileFieldChange}
              />

              <RuntimeSection form={form} onFieldChange={updateField} />

              <MCPSection
                form={form}
                onFieldChange={updateField}
                onAddServer={handleMCPServerAdd}
                onRemoveServer={handleMCPServerRemove}
                onServerFieldChange={handleMCPServerFieldChange}
              />

              <ExecSection form={form} onFieldChange={updateField} />

              <HooksSection
                form={form}
                onFieldChange={updateField}
                onProcessAdd={handleHookProcessAdd}
                onProcessRemove={handleHookProcessRemove}
                onProcessFieldChange={handleHookProcessFieldChange}
                onBuiltinAdd={handleHookBuiltinAdd}
                onBuiltinRemove={handleHookBuiltinRemove}
                onBuiltinToggle={handleHookBuiltinToggle}
              />

              {!isDirty && actionButtons}
            </div>
          )}
        </div>
      </div>
      {isDirty && (
        <div className="border-border/70 bg-background/95 supports-backdrop-filter:bg-background/80 shrink-0 border-t px-3 py-3 shadow-[0_-12px_30px_rgba(15,23,42,0.10)] backdrop-blur lg:px-6">
          <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex-1">
              <ConfigChangeNotice
                kind="save"
                title={t("common.saveChangesTitle")}
                description={t("pages.config.unsaved_changes")}
              />
            </div>
            {actionButtons}
          </div>
        </div>
      )}
    </div>
  )
}
