import { IconPlus, IconTrash } from "@tabler/icons-react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { listBuiltinHooks } from "@/api/hooks"
import { ConfigSectionCard } from "@/components/config/config-sections"
import { Field, SwitchCardField } from "@/components/shared-form"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"

import type { CoreConfigForm, HookProcessForm } from "./form-model"

type UpdateCoreField = <K extends keyof CoreConfigForm>(key: K, value: CoreConfigForm[K]) => void

interface HooksSectionProps {
  form: CoreConfigForm
  onFieldChange: UpdateCoreField
  onProcessAdd: () => void
  onProcessRemove: (id: string) => void
  onProcessFieldChange: <K extends keyof HookProcessForm>(
    id: string,
    key: K,
    value: HookProcessForm[K],
  ) => void
  onBuiltinAdd: (name: string) => void
  onBuiltinRemove: (id: string) => void
  onBuiltinToggle: (id: string, enabled: boolean) => void
}

export function HooksSection({
  form,
  onFieldChange,
  onProcessAdd,
  onProcessRemove,
  onProcessFieldChange,
  onBuiltinAdd,
  onBuiltinRemove,
  onBuiltinToggle,
}: HooksSectionProps) {
  const { t } = useTranslation()
  const builtinsQuery = useQuery({ queryKey: ["hooks", "builtins"], queryFn: listBuiltinHooks })
  const knownBuiltins = builtinsQuery.data ?? []
  const nextBuiltin = knownBuiltins.find(
    (name) => !form.hookBuiltins.some((builtin) => builtin.name === name),
  )

  return (
    <ConfigSectionCard
      title={t("pages.config.sections.hooks")}
      description={t("pages.config.hooks_hint")}
    >
      <SwitchCardField
        label={t("pages.config.hooks_enabled")}
        hint={t("pages.config.hooks_enabled_hint")}
        layout="setting-row"
        checked={form.hooksEnabled}
        onCheckedChange={(checked) => onFieldChange("hooksEnabled", checked)}
      />

      <Field
        label={t("pages.config.hooks_observer_timeout")}
        hint={t("pages.config.hooks_timeout_hint")}
        layout="setting-row"
      >
        <Input
          type="number"
          min={1}
          disabled={!form.hooksEnabled}
          value={form.hooksObserverTimeoutMs}
          onChange={(e) => onFieldChange("hooksObserverTimeoutMs", e.target.value)}
        />
      </Field>

      <Field
        label={t("pages.config.hooks_interceptor_timeout")}
        hint={t("pages.config.hooks_timeout_hint")}
        layout="setting-row"
      >
        <Input
          type="number"
          min={1}
          disabled={!form.hooksEnabled}
          value={form.hooksInterceptorTimeoutMs}
          onChange={(e) => onFieldChange("hooksInterceptorTimeoutMs", e.target.value)}
        />
      </Field>

      <Field
        label={t("pages.config.hooks_approval_timeout")}
        hint={t("pages.config.hooks_approval_timeout_hint")}
        layout="setting-row"
      >
        <Input
          type="number"
          min={1}
          disabled={!form.hooksEnabled}
          value={form.hooksApprovalTimeoutMs}
          onChange={(e) => onFieldChange("hooksApprovalTimeoutMs", e.target.value)}
        />
      </Field>

      <div className="space-y-3 py-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-sm font-medium">{t("pages.config.hooks_builtins")}</p>
            <p className="text-muted-foreground text-xs">{t("pages.config.hooks_builtins_hint")}</p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!form.hooksEnabled || nextBuiltin === undefined}
            onClick={() => onBuiltinAdd(nextBuiltin ?? "")}
          >
            <IconPlus className="size-4" />
            {t("pages.config.hooks_add_builtin")}
          </Button>
        </div>

        {knownBuiltins.length === 0 ? (
          <p className="text-muted-foreground text-xs">{t("pages.config.hooks_no_builtins")}</p>
        ) : null}

        {form.hookBuiltins.map((builtin) => (
          <div
            key={builtin.id}
            className="border-border/60 flex items-center gap-3 rounded-lg border p-3"
          >
            <Switch
              checked={builtin.enabled}
              disabled={!form.hooksEnabled}
              aria-label={builtin.name}
              onCheckedChange={(checked) => onBuiltinToggle(builtin.id, checked)}
            />
            <span className="font-mono text-sm">{builtin.name}</span>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="ml-auto"
              aria-label={t("pages.config.hooks_remove")}
              onClick={() => onBuiltinRemove(builtin.id)}
            >
              <IconTrash className="size-4" />
            </Button>
          </div>
        ))}
      </div>

      <div className="space-y-3 py-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-sm font-medium">{t("pages.config.hooks_processes")}</p>
            <p className="text-muted-foreground text-xs">{t("pages.config.hooks_processes_hint")}</p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!form.hooksEnabled}
            onClick={onProcessAdd}
          >
            <IconPlus className="size-4" />
            {t("pages.config.hooks_add_process")}
          </Button>
        </div>

        {form.hookProcesses.map((process) => (
          <div key={process.id} className="border-border/60 space-y-3 rounded-lg border p-4">
            <div className="flex items-center gap-3">
              <Switch
                checked={process.enabled}
                disabled={!form.hooksEnabled}
                aria-label={t("pages.config.hooks_process_enabled")}
                onCheckedChange={(checked) => onProcessFieldChange(process.id, "enabled", checked)}
              />
              <Input
                value={process.name}
                placeholder={t("pages.config.hooks_process_name")}
                onChange={(e) => onProcessFieldChange(process.id, "name", e.target.value)}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t("pages.config.hooks_remove")}
                onClick={() => onProcessRemove(process.id)}
              >
                <IconTrash className="size-4" />
              </Button>
            </div>

            <Field
              label={t("pages.config.hooks_process_command")}
              hint={t("pages.config.hooks_command_hint")}
            >
              <Input
                value={process.command}
                placeholder="uvx"
                onChange={(e) => onProcessFieldChange(process.id, "command", e.target.value)}
              />
            </Field>

            <Field label={t("pages.config.hooks_process_args")} hint={t("pages.config.hooks_args_hint")}>
              <Input
                value={process.argsText}
                placeholder="mole-hook-reviewer --strict"
                onChange={(e) => onProcessFieldChange(process.id, "argsText", e.target.value)}
              />
            </Field>

            <Field
              label={t("pages.config.hooks_process_intercept")}
              hint={t("pages.config.hooks_intercept_hint")}
            >
              <Textarea
                rows={2}
                value={process.interceptText}
                placeholder="before_tool, approve_tool"
                onChange={(e) => onProcessFieldChange(process.id, "interceptText", e.target.value)}
              />
            </Field>

            <Field
              label={t("pages.config.hooks_process_observe")}
              hint={t("pages.config.hooks_observe_hint")}
            >
              <Textarea
                rows={2}
                value={process.observeText}
                placeholder="turn_start, turn_end"
                onChange={(e) => onProcessFieldChange(process.id, "observeText", e.target.value)}
              />
            </Field>

            <Field label={t("pages.config.hooks_process_env")} hint={t("pages.config.hooks_env_hint")}>
              <Input
                value={process.envText}
                placeholder='{"HOOK_MODE":"rewrite"}'
                onChange={(e) => onProcessFieldChange(process.id, "envText", e.target.value)}
              />
            </Field>

            <Field label={t("pages.config.hooks_process_dir")}>
              <Input
                value={process.dir}
                onChange={(e) => onProcessFieldChange(process.id, "dir", e.target.value)}
              />
            </Field>
          </div>
        ))}
      </div>
    </ConfigSectionCard>
  )
}
