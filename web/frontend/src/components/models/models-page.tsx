import {
  IconArrowsSort,
  IconDatabase,
  IconLoader2,
  IconPlus,
  IconStar,
} from "@tabler/icons-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type ModelInfo,
  type ModelReferenceRename,
  getModels,
  updateDefaultChain,
} from "@/api/models"
import { ConfigChangeNotice } from "@/components/config-change-notice"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { refreshGatewayState } from "@/store/gateway"

import { AddModelSheet } from "./add-model-sheet"
import { CatalogDialog } from "./catalog-dialog"
import { DefaultChainDialog } from "./default-chain-dialog"
import { DeleteModelDialog } from "./delete-model-dialog"
import { EditModelSheet } from "./edit-model-sheet"
import { ProviderSection } from "./provider-section"

interface ProviderGroup {
  key: string
  label: string
  models: ModelInfo[]
  hasDefault: boolean
  availableCount: number
}

function sortModelList(models: ModelInfo[], defaultModelName: string) {
  return [...models].sort((a, b) => {
    const aDefault = a.model_name === defaultModelName
    const bDefault = b.model_name === defaultModelName
    if (aDefault && !bDefault) return -1
    if (!aDefault && bDefault) return 1
    if (a.available && !b.available) return -1
    if (!a.available && b.available) return 1
    return a.model_name.localeCompare(b.model_name)
  })
}

function applyRename(
  names: string[],
  renames: ModelReferenceRename[],
): string[] {
  return names.map((name) => {
    for (const rename of renames) {
      if (name === rename.from) return rename.to
    }
    return name
  })
}

export function ModelsPage() {
  const { t } = useTranslation()
  const [models, setModels] = useState<ModelInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState("")

  const [editingModel, setEditingModel] = useState<ModelInfo | null>(null)
  const [deletingModel, setDeletingModel] = useState<ModelInfo | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [catalogOpen, setCatalogOpen] = useState(false)
  const [defaultChainOpen, setDefaultChainOpen] = useState(false)
  const [defaultModelDraft, setDefaultModelDraft] = useState("")
  const [defaultModelBaseline, setDefaultModelBaseline] = useState("")
  const [fallbackChainDraft, setFallbackChainDraft] = useState<string[]>([])
  const [fallbackChainBaseline, setFallbackChainBaseline] = useState<string[]>(
    [],
  )
  const [savingChain, setSavingChain] = useState(false)
  const chainDirtyRef = useRef(false)
  const pendingRenames = useRef<ModelReferenceRename[]>([])

  const chainDirty =
    defaultModelDraft !== defaultModelBaseline ||
    JSON.stringify(fallbackChainDraft) !== JSON.stringify(fallbackChainBaseline)
  useEffect(() => {
    chainDirtyRef.current = chainDirty
  }, [chainDirty])

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const data = await getModels()
      const persistedDefault = data.default_model || ""
      const persistedChain = data.fallback_chain || []
      const renames = pendingRenames.current
      pendingRenames.current = []

      setModels(sortModelList(data.models, persistedDefault))
      if (chainDirtyRef.current) {
        const aliases = new Set(data.models.map((model) => model.model_name))
        setDefaultModelDraft((current) => {
          const renamed = applyRename([current], renames)[0]
          return aliases.has(renamed) ? renamed : persistedDefault
        })
        setFallbackChainDraft((current) => {
          const renamed = applyRename(current, renames)
          return [...new Set(renamed)].filter(
            (name) => aliases.has(name) && name !== persistedDefault,
          )
        })
      } else {
        setDefaultModelDraft(persistedDefault)
        setDefaultModelBaseline(persistedDefault)
        setFallbackChainDraft(persistedChain)
        setFallbackChainBaseline(persistedChain)
      }
      setFetchError("")
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : t("models.loadError"))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const handleSetDefault = (model: ModelInfo) => {
    if (!model.available || defaultModelDraft === model.model_name) return
    setDefaultModelDraft(model.model_name)
    setFallbackChainDraft((prev) =>
      prev.filter((name) => name !== model.model_name),
    )
  }

  const handleToggleFallback = (model: ModelInfo) => {
    if (
      !model.available ||
      model.model_name === defaultModelDraft ||
      model.is_virtual
    ) {
      return
    }
    setFallbackChainDraft((prev) =>
      prev.includes(model.model_name)
        ? prev.filter((item) => item !== model.model_name)
        : [...prev, model.model_name],
    )
  }

  const moveFallback = (modelName: string, targetIndex: number) => {
    setFallbackChainDraft((prev) => {
      const currentIndex = prev.indexOf(modelName)
      if (currentIndex === -1) return prev
      const next = [...prev]
      next.splice(currentIndex, 1)
      next.splice(targetIndex, 0, modelName)
      return next
    })
  }

  const handleSaveChain = async () => {
    setSavingChain(true)
    try {
      const saved = await updateDefaultChain({
        default_model: defaultModelDraft,
        fallback_chain: fallbackChainDraft,
      })
      const persistedDefault = saved.default_model || ""
      const persistedChain = saved.fallback_chain || []
      setDefaultModelBaseline(persistedDefault)
      setDefaultModelDraft(persistedDefault)
      setFallbackChainBaseline(persistedChain)
      setFallbackChainDraft(persistedChain)
      await refreshGatewayState({ force: true })
      toast.success(t("models.defaultChain.saveSuccess"))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t("models.loadError"))
    } finally {
      setSavingChain(false)
    }
  }

  const handleResetChain = () => {
    setDefaultModelDraft(defaultModelBaseline)
    setFallbackChainDraft(fallbackChainBaseline)
  }

  const grouped = new Map<string, ProviderGroup>()
  for (const model of models) {
    const key = model.provider?.trim() || "openai"
    let group = grouped.get(key)
    if (!group) {
      group = {
        key,
        label: key,
        models: [],
        hasDefault: false,
        availableCount: 0,
      }
      grouped.set(key, group)
    }
    group.models.push(model)
    if (model.model_name === defaultModelDraft) group.hasDefault = true
    if (model.available) group.availableCount++
  }
  const providerGroups = [...grouped.values()].sort((a, b) => {
    if (a.hasDefault !== b.hasDefault) return a.hasDefault ? -1 : 1
    if (a.availableCount !== b.availableCount) {
      return b.availableCount - a.availableCount
    }
    return a.label.localeCompare(b.label)
  })

  const defaultModel = models.find(
    (model) => model.model_name === defaultModelDraft,
  )
  const defaultModelEntryCount = models.filter(
    (model) => model.model_name === defaultModelDraft,
  ).length
  const defaultModelLabel = defaultModel?.model_name ?? defaultModelDraft
  const existingModelNames = models.map((model) => model.model_name)
  const existingProviders = [
    ...new Set(
      models
        .map((model) => model.provider?.trim())
        .filter((provider): provider is string => Boolean(provider)),
    ),
  ].sort()

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.models")}>
        <div className="flex items-center gap-3">
          <Button
            size="sm"
            variant="outline"
            onClick={() => setDefaultChainOpen(true)}
            disabled={models.length === 0 && !defaultModelDraft}
          >
            <IconArrowsSort className="size-4" />
            {t("models.defaultChain.button")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => setCatalogOpen(true)}
          >
            <IconDatabase className="size-4" />
            {t("models.catalog.button")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => setAddOpen(true)}
          >
            <IconPlus className="size-4" />
            {t("models.add.button")}
          </Button>
        </div>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
        <div className="pt-2">
          {!defaultModelDraft && (
            <div className="text-muted-foreground flex items-center gap-1.5 text-sm">
              <span>{t("models.noDefaultHintPrefix")}</span>
              <IconStar className="size-3.5 shrink-0" />
              <span>{t("models.noDefaultHintSuffix")}</span>
            </div>
          )}
          <p className="text-muted-foreground mt-1 text-sm">
            {t("models.description")}
          </p>
          {!loading && (models.length > 0 || defaultModelDraft) && (
            <div className="bg-muted/30 mt-4 rounded-xl border px-4 py-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="text-sm font-medium">
                    {t("models.defaultChain.title")}
                  </p>
                  <p className="text-muted-foreground mt-1 text-sm">
                    {defaultModelLabel
                      ? t("models.defaultChain.summary", {
                          model: defaultModelLabel,
                          count: fallbackChainDraft.length,
                        })
                      : t("models.defaultChain.noDefault")}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setDefaultChainOpen(true)}
                >
                  <IconArrowsSort className="size-4" />
                  {t("models.defaultChain.manage")}
                </Button>
              </div>
            </div>
          )}
        </div>

        {(loading || savingChain) && (
          <div className="flex items-center justify-center py-20">
            <IconLoader2 className="text-muted-foreground size-6 animate-spin" />
          </div>
        )}

        {fetchError && (
          <div className="bg-destructive/10 rounded-lg px-4 py-3 text-sm">
            <p className="text-destructive">{fetchError}</p>
            <div className="mt-3 flex items-center gap-2">
              <Button size="sm" variant="outline" onClick={() => void refresh()}>
                {t("models.retry")}
              </Button>
            </div>
          </div>
        )}

        {!loading && !savingChain && !fetchError && (
          <div className="pb-8">
            {providerGroups.map((providerGroup) => (
              <ProviderSection
                key={providerGroup.key}
                provider={{ key: providerGroup.key, label: providerGroup.label }}
                models={providerGroup.models}
                onEdit={setEditingModel}
                onSetDefault={handleSetDefault}
                onToggleFallback={handleToggleFallback}
                onDelete={setDeletingModel}
                fallbackChain={fallbackChainDraft}
                defaultModelName={defaultModelDraft}
                defaultModelEntryCount={defaultModelEntryCount}
              />
            ))}
          </div>
        )}
      </div>

      <EditModelSheet
        model={editingModel}
        open={editingModel !== null}
        onClose={() => setEditingModel(null)}
        onUpdateStarted={() => {}}
        onUpdated={(referenceRename) => {
          if (referenceRename) pendingRenames.current.push(referenceRename)
        }}
        onSaved={() => {
          void refresh()
        }}
        onUpdateSettled={() => {}}
        existingProviders={existingProviders}
      />

      <AddModelSheet
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onSaved={() => {
          void refresh()
        }}
        existingModelNames={existingModelNames}
        existingProviders={existingProviders}
      />

      <DeleteModelDialog
        model={deletingModel}
        isDefaultModel={
          deletingModel?.model_name === defaultModelDraft &&
          defaultModelEntryCount <= 1
        }
        onClose={() => setDeletingModel(null)}
        onDeleted={() => {
          void refresh()
        }}
      />

      <CatalogDialog
        open={catalogOpen}
        onClose={() => setCatalogOpen(false)}
        onModelAdded={() => {
          void refresh()
        }}
      />

      <DefaultChainDialog
        open={defaultChainOpen}
        onClose={() => setDefaultChainOpen(false)}
        models={models}
        defaultModelName={defaultModelDraft}
        fallbackChain={fallbackChainDraft}
        onMoveUp={(modelName) => {
          const index = fallbackChainDraft.indexOf(modelName)
          if (index > 0) moveFallback(modelName, index - 1)
        }}
        onMoveDown={(modelName) => {
          const index = fallbackChainDraft.indexOf(modelName)
          if (index !== -1 && index < fallbackChainDraft.length - 1) {
            moveFallback(modelName, index + 1)
          }
        }}
        onMoveTop={(modelName) => moveFallback(modelName, 0)}
        onMoveBottom={(modelName) =>
          moveFallback(modelName, Math.max(fallbackChainDraft.length - 1, 0))
        }
        onRemove={(modelName) =>
          setFallbackChainDraft((prev) =>
            prev.filter((item) => item !== modelName),
          )
        }
      />

      {chainDirty && (
        <div className="border-border/70 bg-background/95 supports-backdrop-filter:bg-background/80 shrink-0 border-t px-4 py-3 shadow-[0_-12px_30px_rgba(15,23,42,0.10)] backdrop-blur">
          <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex-1">
              <ConfigChangeNotice
                kind="save"
                title={t("common.saveChangesTitle")}
                description={t("models.defaultChain.unsaved")}
              />
            </div>
            <div className="flex items-center justify-end gap-2">
              <Button
                variant="outline"
                onClick={handleResetChain}
                disabled={savingChain}
              >
                {t("common.reset")}
              </Button>
              <Button onClick={handleSaveChain} disabled={savingChain}>
                {savingChain ? t("common.saving") : t("common.save")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
