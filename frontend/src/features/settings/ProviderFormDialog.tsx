import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2, PlugZap, RefreshCw, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Select } from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { providersApi, type ModelProviderDTO, type TestConnectionResult } from "@/features/settings/api";
import { PROVIDER_PRESETS } from "@/features/settings/model-presets";

/**
 * Add/edit dialog for one LLM provider. Supports prefills for common vendors,
 * a "test connection" probe, fetching the live model list via /models, and
 * manual model entries. The API key is write-only: blank keeps the stored one.
 */
interface ProviderFormDialogProps {
  open: boolean;
  /** null = add mode. */
  provider: ModelProviderDTO | null;
  onClose: () => void;
  onSaved: () => void;
}

export function ProviderFormDialog({ open, provider, onClose, onSaved }: ProviderFormDialogProps) {
  const { t } = useTranslation();
  const isEdit = provider !== null;

  const [name, setName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [keyCleared, setKeyCleared] = useState(false);
  const [models, setModels] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [modelInput, setModelInput] = useState("");
  const [presetSel, setPresetSel] = useState("");
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null);
  const [fetching, setFetching] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  // Sync form state whenever the dialog opens for a different provider.
  useEffect(() => {
    if (!open) return;
    setName(provider?.name ?? "");
    setBaseUrl(provider?.baseUrl ?? "");
    setApiKey("");
    setKeyCleared(false);
    setModels(provider?.models ?? []);
    setEnabled(provider?.enabled ?? true);
    setModelInput("");
    setPresetSel("");
    setTestResult(null);
    setError("");
  }, [open, provider]);

  const applyPreset = (presetName: string) => {
    setPresetSel(presetName);
    const preset = PROVIDER_PRESETS.find((p) => p.name === presetName);
    if (preset) {
      setName(preset.name);
      setBaseUrl(preset.baseUrl);
      setModels(preset.models);
      return;
    }
    // "Custom OpenAI" — self-hosted or third-party OpenAI-compatible endpoint.
    // Undo values autofilled by a previous preset, keep anything the user
    // typed; the model list comes from the endpoint's standard /models API.
    if (PROVIDER_PRESETS.some((p) => p.name === name)) {
      setName(t("settings.models.customName"));
    }
    if (PROVIDER_PRESETS.some((p) => p.baseUrl === baseUrl)) {
      setBaseUrl("");
    }
  };

  const addModel = () => {
    const m = modelInput.trim();
    if (!m || models.includes(m)) {
      setModelInput("");
      return;
    }
    setModels([...models, m]);
    setModelInput("");
  };

  const removeModel = (m: string) => setModels(models.filter((x) => x !== m));

  const handleFetchModels = async () => {
    setFetching(true);
    setError("");
    try {
      const list = (await providersApi.fetchModels({
        id: provider?.id ?? "",
        baseUrl,
        apiKey,
        model: "",
      })) ?? [];
      if (list.length > 0) {
        setModels(list);
        setTestResult(null);
      } else {
        setError(t("settings.models.fetchEmpty"));
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setFetching(false);
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    setError("");
    try {
      const result = await providersApi.test({
        id: provider?.id ?? "",
        baseUrl,
        apiKey,
        model: models[0] ?? "",
      });
      setTestResult(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    if (!name.trim() || !baseUrl.trim()) {
      setError(t("settings.models.formIncomplete"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      await providersApi.save({
        id: provider?.id ?? "",
        name: name.trim(),
        baseUrl: baseUrl.trim(),
        models,
        enabled,
        // Backend convention: "" keeps the stored key, " " clears it.
        apiKey: keyCleared ? " " : apiKey,
      });
      onSaved();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t("settings.models.editProvider") : t("settings.models.addProvider")}
          </DialogTitle>
          <DialogDescription>{t("settings.models.dialogDesc")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {!isEdit && (
            <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
              <Label>{t("settings.models.preset")}</Label>
              <Select value={presetSel} onChange={(e) => applyPreset(e.target.value)}>
                <option value="">{t("settings.models.presetCustom")}</option>
                {PROVIDER_PRESETS.map((p) => (
                  <option key={p.name} value={p.name}>{p.name}</option>
                ))}
              </Select>
            </div>
          )}

          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label>{t("settings.models.providerName")}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="OpenAI" />
          </div>

          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label>{t("settings.models.baseUrl")}</Label>
            <Input
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder={
                !isEdit && !presetSel
                  ? t("settings.models.customUrlPlaceholder")
                  : "https://api.openai.com/v1"
              }
              className="font-mono text-xs"
            />
          </div>

          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label>{t("settings.models.apiKey")}</Label>
            <div className="flex items-center gap-2">
              <Input
                type="password"
                value={apiKey}
                onChange={(e) => {
                  setApiKey(e.target.value);
                  setKeyCleared(false);
                }}
                placeholder={isEdit && provider?.hasApiKey ? t("settings.models.apiKeyKeep") : "sk-…"}
                className="font-mono text-xs"
              />
              {isEdit && provider?.hasApiKey && !apiKey && (
                keyCleared ? (
                  <Badge variant="destructive">{t("settings.models.keyCleared")}</Badge>
                ) : (
                  <button
                    type="button"
                    onClick={() => setKeyCleared(true)}
                    title={t("settings.models.clearKey")}
                    className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                  >
                    <X className="h-4 w-4" />
                  </button>
                )
              )}
            </div>
          </div>

          <div className="grid grid-cols-[8rem_1fr] items-start gap-3">
            <Label className="pt-2">{t("settings.models.modelsTitle")}</Label>
            <div className="flex flex-col gap-2">
              {models.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {models.map((m) => (
                    <Badge key={m} variant="secondary" className="gap-1 font-mono text-xs">
                      {m}
                      <button
                        type="button"
                        onClick={() => removeModel(m)}
                        className="text-muted-foreground hover:text-foreground"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </Badge>
                  ))}
                </div>
              )}
              <div className="flex items-center gap-2">
                <Input
                  value={modelInput}
                  onChange={(e) => setModelInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addModel();
                    }
                  }}
                  placeholder={t("settings.models.addModelPlaceholder")}
                  className="font-mono text-xs"
                />
                <Button type="button" variant="outline" size="sm" onClick={addModel}>
                  {t("settings.models.addModel")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleFetchModels}
                  disabled={fetching || !baseUrl.trim()}
                >
                  {fetching ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RefreshCw className="h-3.5 w-3.5" />
                  )}
                  {t("settings.models.fetchModels")}
                </Button>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-[8rem_1fr] items-center gap-3">
            <Label>{t("settings.models.enabled")}</Label>
            <label className="flex cursor-pointer items-center gap-2 text-sm">
              <Checkbox checked={enabled} onCheckedChange={(v) => setEnabled(v === true)} />
              <span className="text-muted-foreground">{t("settings.models.enabledDesc")}</span>
            </label>
          </div>

          {testResult && (
            <div className="flex items-start gap-2 text-xs">
              <Badge variant={testResult.ok ? "success" : "destructive"}>
                {testResult.ok ? t("settings.models.testOk") : t("settings.models.testFailed")}
              </Badge>
              <span className="min-w-0 flex-1 break-all font-mono text-muted-foreground">
                {testResult.msg}
              </span>
            </div>
          )}
          {error && <p className="break-all text-xs text-destructive">{error}</p>}
        </div>

        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={handleTest} disabled={testing || !baseUrl.trim() || models.length === 0}>
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PlugZap className="h-3.5 w-3.5" />}
            {t("settings.models.test")}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="button" onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
