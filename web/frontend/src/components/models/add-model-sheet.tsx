import {
  IconDownload,
  IconLoader2,
  IconPlugConnected,
} from "@tabler/icons-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { addModel, setDefaultModel } from "@/api/models"
import { ConfigChangeNotice } from "@/components/config-change-notice"
import { maskedSecretPlaceholder } from "@/components/secret-placeholder"
import {
  AdvancedSection,
  Field,
  KeyInput,
  SwitchCardField,
} from "@/components/shared-form"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Textarea } from "@/components/ui/textarea"
import { refreshGatewayState } from "@/store/gateway"

import { FetchModelsDialog } from "./fetch-models-dialog"
import { type FieldValidation, validateModelField } from "./model-validation"
import { ProviderCombobox } from "./provider-combobox"
import { TestModelDialog } from "./test-model-dialog"

interface AddForm {
  modelName: string
  provider: string
  apiBase: string
  apiKey: string
  model: string
  rpm: string
  maxTokensField: string
  requestTimeout: string
  thinkingLevel: string
  toolSchemaTransform: string
  streamingEnabled: boolean
  extraBody: string
  customHeaders: string
}

const EMPTY_ADD_FORM: AddForm = {
  modelName: "",
  provider: "",
  apiBase: "",
  apiKey: "",
  model: "",
  rpm: "",
  maxTokensField: "",
  requestTimeout: "",
  thinkingLevel: "",
  toolSchemaTransform: "",
  streamingEnabled: false,
  extraBody: "",
  customHeaders: "",
}

interface AddModelSheetProps {
  open: boolean
  onClose: () => void
  onSaved: () => void
  onMutationStarted?: () => void
  onMutationSettled?: () => void
  existingModelNames: string[]
  existingProviders: string[]
}

export function AddModelSheet({
  open,
  onClose,
  onSaved,
  onMutationStarted,
  onMutationSettled,
  existingModelNames,
  existingProviders,
}: AddModelSheetProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<AddForm>(EMPTY_ADD_FORM)
  const [saving, setSaving] = useState(false)
  const [setAsDefault, setSetAsDefault] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<
    Partial<Record<keyof AddForm, string>>
  >({})
  const [serverError, setServerError] = useState("")
  const [modelValidation, setModelValidation] =
    useState<FieldValidation | null>(null)
  const [fetchOpen, setFetchOpen] = useState(false)
  const [testOpen, setTestOpen] = useState(false)
  const [fetchedModels, setFetchedModels] = useState<string[]>([])
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  const scrollContainerRef = useRef<HTMLDivElement>(null)

  const apiKeyPlaceholder = maskedSecretPlaceholder(
    form.apiKey,
    t("models.field.apiKeyPlaceholder"),
  )
  const isDirty =
    JSON.stringify(form) !== JSON.stringify(EMPTY_ADD_FORM) || setAsDefault

  useEffect(() => {
    if (open) {
      setForm(EMPTY_ADD_FORM)
      setSetAsDefault(false)
      setFieldErrors({})
      setServerError("")
      setModelValidation(null)
      setFetchedModels([])
    }
  }, [open])

  const validate = (): boolean => {
    const errors: Partial<Record<keyof AddForm, string>> = {}
    const modelName = form.modelName.trim()
    if (!modelName) {
      errors.modelName = t("models.add.errorRequired")
    } else if (existingModelNames.some((name) => name.trim() === modelName)) {
      errors.modelName = t("models.add.errorDuplicateModelName")
    }
    if (!form.provider.trim()) errors.provider = t("models.add.errorRequired")
    if (!form.apiBase.trim()) errors.apiBase = t("models.add.errorRequired")
    if (!form.model.trim()) errors.model = t("models.add.errorRequired")
    if (modelValidation?.level === "error") {
      errors.model = t(
        modelValidation.messageKey,
        modelValidation.messageParams,
      )
    }
    setFieldErrors(errors)
    return Object.keys(errors).length === 0
  }

  const setField =
    (key: keyof AddForm) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setForm((f) => ({ ...f, [key]: e.target.value }))
      if (fieldErrors[key]) {
        setFieldErrors((prev) => ({ ...prev, [key]: undefined }))
      }
    }

  const debouncedValidateModel = useCallback((value: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      setModelValidation(validateModelField(value))
    }, 300)
  }, [])

  const handleModelChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setForm((f) => ({ ...f, model: value }))
    if (fieldErrors.model) {
      setFieldErrors((prev) => ({ ...prev, model: undefined }))
    }
    debouncedValidateModel(value)
  }

  const applyFix = () => {
    if (modelValidation?.fix) {
      setForm((f) => ({ ...f, model: modelValidation.fix! }))
      setModelValidation(null)
    }
  }

  const handleModelPick = (modelId: string) => {
    setForm((f) => ({ ...f, model: modelId }))
    setModelValidation(null)
    if (fieldErrors.model) {
      setFieldErrors((prev) => ({ ...prev, model: undefined }))
    }
  }

  const handleFetchFill = (models: string[]) => {
    setFetchedModels(models)
    if (models.length >= 1) {
      handleModelPick(models[0])
    }
  }

  const handleSave = async () => {
    if (!validate()) return

    let extraBody: Record<string, unknown> | undefined
    let customHeaders: Record<string, string> | undefined
    try {
      extraBody = form.extraBody.trim()
        ? JSON.parse(form.extraBody.trim())
        : {}
    } catch {
      setServerError(
        t("models.field.extraBody") + ": " + t("models.field.invalidJson"),
      )
      return
    }
    try {
      customHeaders = form.customHeaders.trim()
        ? JSON.parse(form.customHeaders.trim())
        : {}
    } catch {
      setServerError(
        t("models.field.customHeaders") + ": " + t("models.field.invalidJson"),
      )
      return
    }

    onMutationStarted?.()
    setSaving(true)
    setServerError("")
    try {
      const modelName = form.modelName.trim()
      await addModel({
        model_name: modelName,
        provider: form.provider.trim(),
        model: form.model.trim(),
        api_base: form.apiBase.trim().replace(/\/+$/, ""),
        api_key: form.apiKey.trim() || undefined,
        rpm: form.rpm ? Number(form.rpm) : undefined,
        max_tokens_field: form.maxTokensField.trim() || undefined,
        request_timeout: form.requestTimeout
          ? Number(form.requestTimeout)
          : undefined,
        thinking_level: form.thinkingLevel.trim() || undefined,
        tool_schema_transform: form.toolSchemaTransform.trim() || undefined,
        streaming: form.streamingEnabled ? { enabled: true } : undefined,
        extra_body: extraBody,
        custom_headers: customHeaders,
      })
      if (setAsDefault) {
        await setDefaultModel(modelName)
      }
      await refreshGatewayState({ force: true })
      toast.success(t("models.add.saveSuccess"))
      onSaved()
      onClose()
    } catch (e) {
      setServerError(e instanceof Error ? e.message : t("models.add.saveError"))
    } finally {
      setSaving(false)
      onMutationSettled?.()
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={(v) => !v && !saving && onClose()}>
        <SheetContent
          side="right"
          className="flex flex-col gap-0 p-0 data-[side=right]:!w-full data-[side=right]:sm:!w-[560px] data-[side=right]:sm:!max-w-[560px]"
        >
          <SheetHeader className="border-b-muted border-b px-6 py-5">
            <SheetTitle className="text-base">
              {t("models.add.title")}
            </SheetTitle>
            <SheetDescription className="text-xs">
              {t("models.add.description")}
            </SheetDescription>
          </SheetHeader>

          <div
            className="min-h-0 flex-1 overflow-y-auto"
            ref={scrollContainerRef}
          >
            <div className="space-y-5 px-6 py-5">
              <Field
                label={t("models.add.modelName")}
                hint={t("models.add.modelNameHint")}
                required
              >
                <Input
                  value={form.modelName}
                  onChange={setField("modelName")}
                  placeholder={t("models.add.modelNamePlaceholder")}
                  aria-invalid={!!fieldErrors.modelName}
                />
                {fieldErrors.modelName && (
                  <p className="text-destructive text-xs">
                    {fieldErrors.modelName}
                  </p>
                )}
              </Field>

              <Field
                label={t("models.field.provider")}
                hint={t("models.field.providerHint")}
                error={fieldErrors.provider}
                required
              >
                <ProviderCombobox
                  value={form.provider}
                  onChange={(provider) =>
                    setForm((f) => ({ ...f, provider }))
                  }
                  placeholder={t("models.field.providerPlaceholder")}
                  suggestions={existingProviders}
                />
              </Field>

              <Field
                label={t("models.field.apiBase")}
                hint={t("models.field.apiBaseHint")}
                error={fieldErrors.apiBase}
                required
              >
                <Input
                  value={form.apiBase}
                  onChange={setField("apiBase")}
                  placeholder="https://api.example.com/v1"
                  className="font-mono text-sm"
                />
              </Field>

              <Field label={t("models.field.apiKey")}>
                <KeyInput
                  value={form.apiKey}
                  onChange={(v) => setForm((f) => ({ ...f, apiKey: v }))}
                  placeholder={apiKeyPlaceholder}
                />
              </Field>

              <Field
                label={t("models.add.modelId")}
                hint={t("models.add.modelIdHint")}
                error={fieldErrors.model}
                required
              >
                <Input
                  value={form.model}
                  onChange={handleModelChange}
                  placeholder={t("models.add.modelIdPlaceholder")}
                  className="font-mono text-sm"
                  aria-invalid={
                    !!fieldErrors.model || modelValidation?.level === "error"
                  }
                />
                {modelValidation && modelValidation.messageKey && (
                  <div
                    className={`flex items-center gap-2 text-xs ${
                      modelValidation.level === "error"
                        ? "text-destructive"
                        : "text-yellow-600 dark:text-yellow-500"
                    }`}
                  >
                    <span>
                      {t(
                        modelValidation.messageKey,
                        modelValidation.messageParams,
                      )}
                    </span>
                    {modelValidation.fix && (
                      <button
                        type="button"
                        onClick={applyFix}
                        className="text-primary underline hover:no-underline"
                      >
                        {t("common.fix")}
                      </button>
                    )}
                  </div>
                )}
                {fetchedModels.length > 0 && (
                  <div className="flex flex-wrap gap-1.5">
                    {fetchedModels.map((m) => (
                      <Badge
                        key={m}
                        variant={form.model === m ? "default" : "outline"}
                        className="cursor-pointer font-mono text-xs"
                        onClick={() => handleModelPick(m)}
                      >
                        {m}
                      </Badge>
                    ))}
                  </div>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs"
                  disabled={!form.apiBase.trim()}
                  onClick={() => setFetchOpen(true)}
                >
                  <IconDownload className="size-3" />
                  {t("models.fetch.title")}
                </Button>
              </Field>

              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setTestOpen(true)}
                  disabled={!form.apiBase.trim() || !form.model.trim()}
                >
                  <IconPlugConnected className="size-4" />
                  {t("models.test.testConnection")}
                </Button>
              </div>

              <SwitchCardField
                label={t("models.defaultOnSave.label")}
                hint={t("models.defaultOnSave.description")}
                checked={setAsDefault}
                onCheckedChange={setSetAsDefault}
              />

              <AdvancedSection>
                <Field
                  label={t("models.field.requestTimeout")}
                  hint={t("models.field.requestTimeoutHint")}
                >
                  <Input
                    value={form.requestTimeout}
                    onChange={setField("requestTimeout")}
                    placeholder="60"
                    type="number"
                    min={0}
                  />
                </Field>

                <Field
                  label={t("models.field.rpm")}
                  hint={t("models.field.rpmHint")}
                >
                  <Input
                    value={form.rpm}
                    onChange={setField("rpm")}
                    placeholder="60"
                    type="number"
                    min={0}
                  />
                </Field>

                <Field
                  label={t("models.field.thinkingLevel")}
                  hint={t("models.field.thinkingLevelHint")}
                >
                  <Input
                    value={form.thinkingLevel}
                    onChange={setField("thinkingLevel")}
                    placeholder={t("models.field.providerDefault")}
                  />
                </Field>

                <Field
                  label={t("models.field.maxTokensField")}
                  hint={t("models.field.maxTokensFieldHint")}
                >
                  <Input
                    value={form.maxTokensField}
                    onChange={setField("maxTokensField")}
                    placeholder="max_completion_tokens"
                  />
                </Field>

                <Field
                  label={t("models.field.toolSchemaTransform")}
                  hint={t("models.field.toolSchemaTransformHint")}
                >
                  <Input
                    value={form.toolSchemaTransform}
                    onChange={setField("toolSchemaTransform")}
                    placeholder="google"
                  />
                </Field>

                <SwitchCardField
                  label={t("models.field.streamingEnabled")}
                  hint={t("models.field.streamingEnabledHint")}
                  checked={form.streamingEnabled}
                  onCheckedChange={(checked) =>
                    setForm((f) => ({ ...f, streamingEnabled: checked }))
                  }
                  ariaLabel={t("models.field.streamingEnabled")}
                />

                <Field
                  label={t("models.field.extraBody")}
                  hint={t("models.field.extraBodyHint")}
                >
                  <Textarea
                    value={form.extraBody}
                    onChange={setField("extraBody")}
                    placeholder='{"key": "value"}'
                    rows={3}
                  />
                </Field>

                <Field
                  label={t("models.field.customHeaders")}
                  hint={t("models.field.customHeadersHint")}
                >
                  <Textarea
                    value={form.customHeaders}
                    onChange={setField("customHeaders")}
                    placeholder='{"X-Source": "coding-plan"}'
                    rows={3}
                  />
                </Field>
              </AdvancedSection>

              {serverError && (
                <p className="text-destructive bg-destructive/10 rounded-md px-3 py-2 text-sm">
                  {serverError}
                </p>
              )}
            </div>
          </div>

          <SheetFooter className="border-t-muted border-t px-6 py-4">
            {isDirty && (
              <ConfigChangeNotice
                kind="save"
                title={t("common.saveChangesTitle")}
                description={t("models.unsavedPrompt")}
              />
            )}
            <Button variant="ghost" onClick={onClose} disabled={saving}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleSave}
              disabled={
                !isDirty || saving || modelValidation?.level === "error"
              }
            >
              {saving && <IconLoader2 className="size-4 animate-spin" />}
              {t("models.add.confirm")}
            </Button>
          </SheetFooter>
        </SheetContent>

        <FetchModelsDialog
          open={fetchOpen}
          onClose={() => setFetchOpen(false)}
          onFill={handleFetchFill}
          provider={form.provider}
          apiKey={form.apiKey}
          apiBase={form.apiBase}
        />

        <TestModelDialog
          model={null}
          open={testOpen}
          onClose={() => setTestOpen(false)}
          inlineParams={{
            provider: form.provider,
            model: form.model,
            apiBase: form.apiBase,
            apiKey: form.apiKey,
          }}
        />
      </Sheet>
    </>
  )
}
