import { useTranslation } from "react-i18next"

import type { ChannelConfig } from "@/api/channels"
import {
  type ArrayFieldFlusher,
  ChannelArrayListField,
} from "@/components/channels/channel-array-list-field"
import { asStringArray, parseAllowFromInput } from "@/components/channels/channel-array-utils"
import { getSecretInputPlaceholder } from "@/components/channels/channel-config-fields"
import { Field, KeyInput, SwitchCardField } from "@/components/shared-form"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

interface QqFormProps {
  config: ChannelConfig
  onChange: (key: string, value: unknown) => void
  configuredSecrets: string[]
  fieldErrors?: Record<string, string>
  registerArrayFieldFlusher?: (
    fieldPath: string,
    flusher: ArrayFieldFlusher | null,
  ) => void
  arrayFieldResetVersion?: number
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function asBool(value: unknown): boolean {
  return typeof value === "boolean" ? value : false
}

export function QqForm({
  config,
  onChange,
  configuredSecrets,
  fieldErrors = {},
  registerArrayFieldFlusher,
  arrayFieldResetVersion,
}: QqFormProps) {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      <Card className="py-3 shadow-sm">
        <CardContent className="divide-border/60 divide-y px-6 py-0 [&>div]:py-5">
          <Field
            label={t("channels.field.appId")}
            required
            hint={t("channels.form.desc.appId")}
            error={fieldErrors.app_id}
          >
            <Input
              value={asString(config.app_id)}
              onChange={(e) => onChange("app_id", e.target.value)}
              placeholder="102xxxxxx"
            />
          </Field>

          <Field
            label={t("channels.field.appSecret")}
            required
            hint={t("channels.form.desc.appSecret")}
            error={fieldErrors.app_secret}
          >
            <KeyInput
              value={asString(config._app_secret)}
              onChange={(v) => onChange("_app_secret", v)}
              placeholder={getSecretInputPlaceholder(
                configuredSecrets,
                "app_secret",
                t("channels.field.secretHintSet"),
                t("channels.field.secretPlaceholder"),
              )}
            />
          </Field>

          <Field
            label={t("channels.field.maxMessageLength")}
            hint={t("channels.form.desc.maxMessageLength")}
          >
            <Input
              type="number"
              min={1}
              value={String(config.max_message_length ?? "")}
              onChange={(e) => {
                const value = e.target.value
                onChange("max_message_length", value === "" ? 0 : Number(value))
              }}
            />
          </Field>

          <Field
            label={t("channels.field.maxBase64FileSizeMiB")}
            hint={t("channels.form.desc.maxBase64FileSizeMiB")}
          >
            <Input
              type="number"
              min={0}
              value={String(config.max_base64_file_size_mib ?? "")}
              onChange={(e) => {
                const value = e.target.value
                onChange("max_base64_file_size_mib", value === "" ? 0 : Number(value))
              }}
            />
          </Field>

          <div>
            <SwitchCardField
              label={t("channels.field.sendMarkdown")}
              hint={t("channels.form.desc.sendMarkdown")}
              checked={asBool(config.send_markdown)}
              onCheckedChange={(checked) => onChange("send_markdown", checked)}
              ariaLabel={t("channels.field.sendMarkdown")}
            />
          </div>
        </CardContent>
      </Card>

      <Card className="py-3 shadow-sm">
        <CardContent className="divide-border/60 divide-y px-6 py-0 [&>div]:py-5">
          <ChannelArrayListField
            label={t("channels.field.allowFrom")}
            hint={t("channels.form.desc.allowFrom")}
            value={asStringArray(config.allow_from)}
            onChange={(value) => onChange("allow_from", value)}
            placeholder={t("channels.field.allowFromPlaceholder")}
            parser={parseAllowFromInput}
            error={fieldErrors.allow_from}
            fieldPath="allow_from"
            registerFlusher={registerArrayFieldFlusher}
            resetVersion={arrayFieldResetVersion}
          />
        </CardContent>
      </Card>
    </div>
  )
}
