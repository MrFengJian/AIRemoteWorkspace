import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialogs } from "@wailsio/runtime";
import { Download, FilePlus, KeyRound, Loader2, Import, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import {
  KeyManagerService,
  type GenerateKeyRequestDTO,
  type ManagedKeyDTO,
} from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useConfirm } from "@/lib/useConfirm";
import { toast, errorMessage } from "@/lib/toast";

const KEYS_KEY = ["managed-keys"] as const;

/**
 * KeysSection — the 密钥管理器 (settings → 密钥): SSH keys stored in the data
 * directory. Generate (Ed25519 / RSA / ECDSA, optional passphrase), import
 * existing private key files (encrypted ones prompt for their passphrase),
 * export (private + .pub), delete. Every action persists immediately.
 */
export function KeysSection() {
  const { t } = useTranslation();
  const { askConfirm } = useConfirm();
  const queryClient = useQueryClient();

  const keys = useQuery({
    queryKey: KEYS_KEY,
    queryFn: () => KeyManagerService.ListKeys().then((r) => r ?? []),
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: KEYS_KEY });

  const [generateOpen, setGenerateOpen] = useState(false);
  const [pendingImport, setPendingImport] = useState<{ path: string } | null>(null);

  const doImport = async (path: string, pass: string): Promise<ManagedKeyDTO> => {
    const key = await KeyManagerService.ImportKey({ name: "", path, passphrase: pass });
    refresh();
    toast.success(t("keys.imported", { name: key.name }));
    return key;
  };

  const handleImportPick = async () => {
    const picked = await Dialogs.OpenFile({ CanChooseFiles: true, Title: t("keys.importPick") });
    if (!picked) return;
    const path = Array.isArray(picked) ? picked[0] : picked;
    try {
      await doImport(path, "");
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("加密")) {
        // Encrypted key — surface the passphrase dialog for a second try.
        setPendingImport({ path });
      } else {
        toast.error(`${t("keys.importFailed")}: ${msg}`);
      }
    }
  };

  const handleGenerate = async (req: GenerateKeyRequestDTO) => {
    // The binding returns [ManagedKeyDTO, publicKeyLine].
    const [key, pubkey] = await KeyManagerService.GenerateKey(req);
    toast.success(
      `${t("keys.generated", { name: key.name })}  ${pubkey ? `(${pubkey.trim()})` : ""}`,
    );
    refresh();
  };

  const exportKey = async (key: ManagedKeyDTO) => {
    const target = await Dialogs.SaveFile({
      Filename: `${key.name}_private`,
    });
    if (!target) return;
    const pubPath = await KeyManagerService.ExportKey(key.id, target);
    toast.success(t("keys.exported", { path: target, pub: pubPath || `${target}.pub` }));
  };

  const deleteKey = async (key: ManagedKeyDTO) => {
    const ok = await askConfirm({
      title: t("keys.deleteTitle"),
      message: t("keys.deleteMsg", { name: key.name }),
      danger: true,
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await KeyManagerService.DeleteKey(key.id);
      refresh();
    } catch (e) {
      toast.error(`${t("keys.deleteFailed")}: ${errorMessage(e)}`);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">{t("keys.title")}</h2>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("keys.title")}</CardTitle>
          <CardDescription>{t("keys.desc")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => setGenerateOpen(true)}
            >
              <FilePlus className="h-3.5 w-3.5" />
              {t("keys.generate")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => void handleImportPick()}
            >
              <Import className="h-3.5 w-3.5" />
              {t("keys.import")}
            </Button>
          </div>

          {keys.isLoading ? (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : (keys.data ?? []).length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">{t("keys.empty")}</p>
          ) : (
            <div className="flex flex-col gap-1.5">
              {(keys.data ?? []).map((k) => (
                <div
                  key={k.id}
                  className="flex items-center gap-2 rounded-[var(--radius)] border border-border px-2.5 py-2"
                >
                  <KeyRound className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-xs font-medium">
                      <span className="truncate">{k.name}</span>
                      <Badge variant="outline" className="shrink-0 text-[10px]">{k.algorithm}</Badge>
                      {k.encrypted && (
                        <Badge variant="secondary" className="shrink-0 text-[10px]">
                          {t("keys.encrypted")}
                        </Badge>
                      )}
                    </div>
                    <div className="truncate font-mono text-[11px] text-muted-foreground">
                      {k.fingerprint} · {t("keys.createdAt", { time: k.createdAt })}
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0"
                    onClick={() =>
                      void (async () => {
                        try {
                          await exportKey(k);
                        } catch (e) {
                          toast.error(`${t("keys.exportFailed")}: ${errorMessage(e)}`);
                        }
                      })()
                    }
                    title={t("keys.export")}
                  >
                    <Download className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0 text-destructive hover:text-destructive"
                    onClick={() => void deleteKey(k)}
                    title={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {generateOpen && (
        <GenerateKeyDialog
          onClose={() => setGenerateOpen(false)}
          onGenerate={async (req) => {
            await handleGenerate(req);
            setGenerateOpen(false);
          }}
        />
      )}

      {pendingImport && (
        <ImportPassDialog
          path={pendingImport.path}
          onClose={() => setPendingImport(null)}
          onConfirm={async (pass) => {
            try {
              await doImport(pendingImport.path, pass);
              setPendingImport(null);
            } catch (e) {
              toast.error(`${t("keys.importFailed")}: ${errorMessage(e)}`);
            }
          }}
        />
      )}
    </div>
  );
}

/** GenerateKeyDialog — name / algorithm / bits / passphrase / comment. */
function GenerateKeyDialog({
  onGenerate,
  onClose,
}: {
  onGenerate: (req: GenerateKeyRequestDTO) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [algorithm, setAlgorithm] = useState("ed25519");
  const [bits, setBits] = useState(4096);
  const [passphrase, setPassphrase] = useState("");
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            <KeyRound className="h-4 w-4 text-primary" /> {t("keys.generate")}
          </DialogTitle>
          <DialogDescription>{t("keys.generateDesc")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="keyName">{t("keys.keyName")}</Label>
            <Input id="keyName" value={name} onChange={(e) => setName(e.target.value)} placeholder="deploy-key" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="keyAlgo">{t("keys.algorithm")}</Label>
              <Select id="keyAlgo" value={algorithm} onChange={(e) => setAlgorithm(e.target.value)}>
                <option value="ed25519">Ed25519</option>
                <option value="rsa">RSA</option>
                <option value="ecdsa">ECDSA P-256</option>
              </Select>
            </div>
            {algorithm === "rsa" && (
              <div className="grid gap-1.5">
                <Label htmlFor="keyBits">{t("keys.bits")}</Label>
                <Select id="keyBits" value={String(bits)} onChange={(e) => setBits(Number(e.target.value))}>
                  <option value="2048">2048</option>
                  <option value="3072">3072</option>
                  <option value="4096">4096</option>
                </Select>
              </div>
            )}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="keyComment">{t("keys.comment")}</Label>
            <Input id="keyComment" value={comment} onChange={(e) => setComment(e.target.value)} placeholder="user@host" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="keyPass">{t("keys.passphrase")}</Label>
            <Input
              id="keyPass"
              type="password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              placeholder={t("keys.passPlaceholder")}
            />
            <p className="text-[11px] text-muted-foreground">{t("keys.passHint")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            className="bg-primary text-primary-foreground"
            disabled={!name.trim() || busy}
            onClick={async () => {
              setBusy(true);
              try {
                await onGenerate({ name: name.trim(), algorithm, bits, passphrase, comment });
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}
            {t("keys.generate")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** ImportPassDialog — passphrase prompt for an encrypted private key. */
function ImportPassDialog({
  path,
  onConfirm,
  onClose,
}: {
  path: string;
  onConfirm: (passphrase: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [pass, setPass] = useState("");
  const [busy, setBusy] = useState(false);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-base">{t("keys.importPassTitle")}</DialogTitle>
          <DialogDescription className="break-all">{path}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="importPass">{t("keys.passphrase")}</Label>
          <Input
            id="importPass"
            type="password"
            value={pass}
            onChange={(e) => setPass(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && pass) void onConfirm(pass);
            }}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            className="bg-primary text-primary-foreground"
            disabled={busy || !pass}
            onClick={async () => {
              setBusy(true);
              await onConfirm(pass);
              setBusy(false);
            }}
          >
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
