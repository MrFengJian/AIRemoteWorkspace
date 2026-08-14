import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bot, Loader2, Pencil, PlugZap, Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useConfirm } from "@/lib/useConfirm";
import {
  providersApi,
  type ModelProviderDTO,
  type TestConnectionResult,
} from "@/features/settings/api";
import { ProviderFormDialog } from "@/features/settings/ProviderFormDialog";

/**
 * Settings → Models. Lists the configured LLM providers with enable toggles,
 * inline testing, and add/edit/delete via ProviderFormDialog. API keys live in
 * the OS vault and never round-trip to the UI.
 */
export function ModelSettingsSection() {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();

  const [providers, setProviders] = useState<ModelProviderDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ModelProviderDTO | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, TestConnectionResult>>({});
  const [busyId, setBusyId] = useState<string | null>(null);

  const reload = useCallback(() => {
    providersApi
      .list()
      .then((list) => setProviders(list ?? []))
      .catch(() => setProviders([]))
      .finally(() => setLoaded(true));
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const openAdd = () => {
    setEditing(null);
    setDialogOpen(true);
  };

  const openEdit = (p: ModelProviderDTO) => {
    setEditing(p);
    setDialogOpen(true);
  };

  const handleTest = async (p: ModelProviderDTO) => {
    setTestingId(p.id);
    try {
      const result = await providersApi.test({ id: p.id, baseUrl: p.baseUrl, apiKey: "", model: p.models?.[0] ?? "" });
      setTestResults((prev) => ({ ...prev, [p.id]: result }));
    } catch (e) {
      setTestResults((prev) => ({
        ...prev,
        [p.id]: { ok: false, msg: e instanceof Error ? e.message : String(e) },
      }));
    } finally {
      setTestingId(null);
    }
  };

  const handleToggle = async (p: ModelProviderDTO, enabled: boolean) => {
    setBusyId(p.id);
    try {
      await providersApi.save({
        id: p.id,
        name: p.name,
        baseUrl: p.baseUrl,
        models: p.models ?? [],
        enabled,
        apiKey: "", // keep the stored key
      });
      reload();
    } finally {
      setBusyId(null);
    }
  };

  const handleDelete = async (p: ModelProviderDTO) => {
    const ok = await askConfirm({
      title: t("settings.models.deleteTitle"),
      message: t("settings.models.deleteConfirm", { name: p.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    setBusyId(p.id);
    try {
      await providersApi.remove(p.id);
      reload();
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{t("settings.models.title")}</h2>
        <Button size="sm" onClick={openAdd}>
          <Plus className="h-3.5 w-3.5" />
          {t("settings.models.addProvider")}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("settings.models.providers")}</CardTitle>
          <CardDescription>{t("settings.models.providersDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {!loaded ? (
            <div className="flex items-center justify-center py-8 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          ) : providers.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-8 text-center">
              <Bot className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("settings.models.empty")}</p>
              <Button size="sm" variant="outline" onClick={openAdd}>
                <Plus className="h-3.5 w-3.5" />
                {t("settings.models.addProvider")}
              </Button>
            </div>
          ) : (
            providers.map((p) => {
              const result = testResults[p.id];
              return (
                <div
                  key={p.id}
                  className="flex flex-col gap-2 rounded-[var(--radius)] border border-border p-3"
                >
                  <div className="flex items-center gap-2">
                    <Checkbox
                      checked={p.enabled}
                      onCheckedChange={(v) => handleToggle(p, v === true)}
                      disabled={busyId === p.id}
                      title={t("settings.models.enabled")}
                    />
                    <span className="text-sm font-medium">{p.name}</span>
                    {!p.enabled && (
                      <Badge variant="secondary">{t("settings.models.disabled")}</Badge>
                    )}
                    {!p.hasApiKey && (
                      <Badge variant="warning">{t("settings.models.noKey")}</Badge>
                    )}
                    <span className="ml-auto font-mono text-xs text-muted-foreground">
                      {t("settings.models.modelsCount", { count: p.models?.length ?? 0 })}
                    </span>
                  </div>
                  <p className="truncate font-mono text-xs text-muted-foreground">{p.baseUrl}</p>
                  {result && (
                    <div className="flex items-start gap-2 text-xs">
                      <Badge variant={result.ok ? "success" : "destructive"}>
                        {result.ok ? t("settings.models.testOk") : t("settings.models.testFailed")}
                      </Badge>
                      <span className="min-w-0 flex-1 break-all font-mono text-muted-foreground">
                        {result.msg}
                      </span>
                    </div>
                  )}
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7"
                      onClick={() => handleTest(p)}
                      disabled={testingId === p.id}
                    >
                      {testingId === p.id ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <PlugZap className="h-3.5 w-3.5" />
                      )}
                      {t("settings.models.test")}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7"
                      onClick={() => openEdit(p)}
                      disabled={busyId === p.id}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                      {t("common.edit")}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 text-destructive hover:text-destructive"
                      onClick={() => handleDelete(p)}
                      disabled={busyId === p.id}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t("common.delete")}
                    </Button>
                  </div>
                </div>
              );
            })
          )}
        </CardContent>
      </Card>

      <ProviderFormDialog
        open={dialogOpen}
        provider={editing}
        onClose={() => setDialogOpen(false)}
        onSaved={reload}
      />
    </div>
  );
}
