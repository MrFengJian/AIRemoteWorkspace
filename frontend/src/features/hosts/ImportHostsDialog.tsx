import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { FolderInput, KeyRound, Loader2 } from "lucide-react";
import { Dialogs } from "@wailsio/runtime";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Select } from "@/components/ui/select";
import { toast, errorMessage } from "@/lib/toast";
import {
  IMPORT_SOURCES,
  hostsApi,
  type ImportSource,
  type ImportedHostDTO,
} from "@/features/hosts/api";
import { HOSTS_KEY } from "@/features/hosts/hooks";

/**
 * ImportHostsDialog — "import hosts from another SSH client" flow:
 * pick a source (Xshell / MobaXterm / PuTTY / SSH config), parse the
 * sessions into a preview checklist, then import the selected rows.
 * Settings only — passwords and key material are never imported; imported
 * hosts simply ask for credentials on first connect.
 */
export function ImportHostsDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [source, setSource] = useState<ImportSource>("xshell");
  const [rows, setRows] = useState<ImportedHostDTO[] | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState<null | "parsing" | "importing">(null);
  // Path the preview was parsed from; "" for the pathless sources
  // (putty → registry, sshconfig → ~/.ssh/config default).
  const [pickedPath, setPickedPath] = useState("");

  const reset = () => {
    setRows(null);
    setSelected(new Set());
  };

  /** A row is importable when it satisfies the host requirements
   *  (mirrors the backend's ImportedHost.valid — missing username is the
   *  common gap, since Xshell/PuTTY sessions may leave it empty). */
  const importable = (r: ImportedHostDTO) =>
    r.host !== "" && r.username !== "" && r.port >= 1 && r.port <= 65535;

  const applyParsed = (parsed: ImportedHostDTO[] | null) => {
    setRows(parsed);
    setSelected(
      new Set((parsed ?? []).filter(importable).map((r) => r.index)),
    );
  };

  const runParse = async (path: string) => {
    setBusy("parsing");
    try {
      const parsed = (await hostsApi.parseImport(source, path)) ?? [];
      if (parsed.length === 0) {
        toast.error(t("hosts.import.empty"));
        return;
      }
      applyParsed(parsed);
    } catch (e) {
      toast.error(`${t("hosts.import.parseFailed")}: ${errorMessage(e)}`);
    } finally {
      setBusy(null);
    }
  };

  /** Browse for a session location manually. On Windows the file and folder
   *  pickers are mutually exclusive (wails forces a folder-only dialog), so
   *  the dialog type follows the source: xshell → its Sessions folder,
   *  mobaxterm → a .mxtsessions export / MobaXterm.ini file. */
  const browseAndParse = async () => {
    const picked = await Dialogs.OpenFile(
      source === "xshell"
        ? { CanChooseDirectories: true, Title: t("hosts.import.pickDirTitle") }
        : { CanChooseFiles: true, Title: t("hosts.import.pickFileTitle") },
    );
    if (!picked) return;
    const path = Array.isArray(picked) ? picked[0] : picked;
    setPickedPath(path);
    await runParse(path);
  };

  /** Auto-detect the tool's default session store and traverse all of it
   *  (xshell: newest Sessions dir; mobaxterm: %APPDATA% MobaXterm.ini;
   *  putty: registry; sshconfig: ~/.ssh/config). */
  const autoParse = async () => {
    setPickedPath("");
    await runParse("");
  };

  const toggle = (index: number, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(index);
      else next.delete(index);
      return next;
    });
  };

  const setAll = (on: boolean) => {
    if (!rows) return;
    setSelected(on ? new Set(rows.filter(importable).map((r) => r.index)) : new Set());
  };

  const doImport = async () => {
    setBusy("importing");
    try {
      // The backend re-parses source+path statelessly; rows/indexes stay in
      // sync because the parse is deterministic for the same input.
      const res = await hostsApi.importHosts(source, pickedPath, [...selected]);
      toast.success(
        t("hosts.import.done", {
          imported: res.imported,
          skipped: res.skipped,
          failed: res.failed,
        }),
      );
      queryClient.invalidateQueries({ queryKey: HOSTS_KEY });
      onOpenChange(false);
    } catch (e) {
      toast.error(`${t("hosts.import.parseFailed")}: ${errorMessage(e)}`);
    } finally {
      setBusy(null);
    }
  };

  const parseButtons =
    source === "xshell" || source === "mobaxterm" ? (
      <>
        <Button
          size="sm"
          variant="outline"
          disabled={busy !== null}
          onClick={() => void browseAndParse()}
        >
          {source === "xshell"
            ? t("hosts.import.pickDir")
            : t("hosts.import.pick")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={busy !== null}
          onClick={() => void autoParse()}
        >
          {busy === "parsing" ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <FolderInput className="h-3.5 w-3.5" />
          )}
          {t("hosts.import.autoScan")}
        </Button>
      </>
    ) : (
      <Button
        size="sm"
        variant="outline"
        disabled={busy !== null}
        onClick={() => void autoParse()}
      >
        {busy === "parsing" ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <FolderInput className="h-3.5 w-3.5" />
        )}
        {t("hosts.import.parse")}
      </Button>
    );

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) reset();
        onOpenChange(v);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("hosts.import.title")}</DialogTitle>
          <DialogDescription>{t("hosts.import.description")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          <div className="flex items-end gap-2">
            <label className="flex min-w-0 flex-1 flex-col gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">
                {t("hosts.import.source")}
              </span>
              <Select
                value={source}
                disabled={busy !== null}
                onChange={(e) => {
                  setSource(e.target.value as ImportSource);
                  setPickedPath("");
                  reset();
                }}
              >
                {IMPORT_SOURCES.map((s) => (
                  <option key={s} value={s}>
                    {t(`hosts.import.source_${s}`)}
                  </option>
                ))}
              </Select>
            </label>
            {parseButtons}
          </div>

          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t(`hosts.import.hint_${source}`)}
          </p>

          {rows && rows.length > 0 && (
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  {t("hosts.import.found", { count: rows.length })}
                </span>
                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 px-2 text-[11px]"
                    onClick={() => setAll(true)}
                  >
                    {t("hosts.import.selectAll")}
                  </Button>
                </div>
              </div>
              <ul className="max-h-64 overflow-auto rounded-[var(--radius)] border border-border">
                {rows.map((r) => {
                  const ok = importable(r);
                  return (
                    <li
                      key={r.index}
                      className="flex items-center gap-2 border-b border-border/60 px-2.5 py-1.5 last:border-b-0"
                    >
                      <Checkbox
                        disabled={!ok}
                        checked={selected.has(r.index)}
                        onCheckedChange={(v) => toggle(r.index, v === true)}
                        aria-label={r.name}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-xs font-medium">
                          {r.name}
                        </span>
                        <span className="block truncate font-mono text-[10px] text-muted-foreground">
                          {r.username ? `${r.username}@` : ""}
                          {r.host}:{r.port}
                          {!ok && (
                            <span className="ml-1.5 font-sans text-warning">
                              {t("hosts.import.invalidRow")}
                            </span>
                          )}
                        </span>
                      </span>
                      {r.group && (
                        <Badge variant="secondary" className="shrink-0 text-[10px]">
                          {r.group}
                        </Badge>
                      )}
                      <Badge
                        variant="outline"
                        className="shrink-0 gap-1 text-[10px] font-normal"
                      >
                        {r.authType === "key" && (
                          <KeyRound className="h-2.5 w-2.5" />
                        )}
                        {t(`hosts.authType.${r.authType || "password"}`)}
                      </Badge>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy !== null || selected.size === 0}
            onClick={() => void doImport()}
          >
            {busy === "importing" && (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            )}
            {t("hosts.import.importSelected", { count: selected.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
