import { useState } from "react";
import { FolderTree, X } from "lucide-react";
import { useTranslation } from "react-i18next";

import { SftpView } from "@/features/sftp/SftpView";
import { cn } from "@/lib/utils";

/**
 * SessionSidebar — the left collapsible panel inside the session page.
 *
 * Designed for extensibility: it holds a row of panel tabs at the top (SFTP
 * today, system-info / processes / etc. in the future) and renders the active
 * panel below. The parent controls visibility; this component just lays out
 * the tab strip + content.
 */
interface SessionSidebarProps {
  hostID: string;
  hostName: string;
  onClose: () => void;
}

type PanelId = "sftp";

export function SessionSidebar({ hostID, hostName, onClose }: SessionSidebarProps) {
  const { t } = useTranslation();
  const [active, setActive] = useState<PanelId>("sftp");

  const tabs: { id: PanelId; label: string; icon: typeof FolderTree }[] = [
    { id: "sftp", label: t("nav.files"), icon: FolderTree },
  ];

  return (
    <div className="flex h-full w-72 shrink-0 flex-col border-r border-border bg-card">
      {/* Tab strip */}
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
        {tabs.map((tab) => {
          const Icon = tab.icon;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActive(tab.id)}
              className={cn(
                "flex h-7 items-center gap-1.5 rounded-[var(--radius)] px-2.5 text-xs transition-colors",
                active === tab.id
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-accent/50",
              )}
            >
              <Icon className="h-3.5 w-3.5" />
              {tab.label}
            </button>
          );
        })}
        <button
          type="button"
          onClick={onClose}
          className="ml-auto rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          title={t("common.close")}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Panel content */}
      <div className="min-h-0 flex-1 overflow-hidden">
        {active === "sftp" && (
          <SftpView embeddedHostID={hostID} embeddedHostName={hostName} />
        )}
      </div>
    </div>
  );
}
