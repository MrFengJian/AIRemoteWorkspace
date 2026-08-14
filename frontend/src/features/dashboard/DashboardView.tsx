import {
  Bot,
  FolderTree,
  Server,
  TerminalSquare,
  ArrowRight,
  type LucideIcon,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useUIStore, type AppView } from "@/stores/ui.store";

interface RoadmapEntry {
  view: AppView;
  titleKey: string;
  descKey: string;
  phaseKey: string;
  icon: LucideIcon;
}

const ROADMAP: RoadmapEntry[] = [
  {
    view: "hosts",
    titleKey: "dashboard.hostManagement.title",
    descKey: "dashboard.hostManagement.desc",
    phaseKey: "dashboard.hostManagement.phase",
    icon: Server,
  },
  {
    view: "terminal",
    titleKey: "dashboard.sshTerminal.title",
    descKey: "dashboard.sshTerminal.desc",
    phaseKey: "dashboard.sshTerminal.phase",
    icon: TerminalSquare,
  },
  {
    view: "sftp",
    titleKey: "dashboard.fileWorkspace.title",
    descKey: "dashboard.fileWorkspace.desc",
    phaseKey: "dashboard.fileWorkspace.phase",
    icon: FolderTree,
  },
  {
    view: "terminal",
    titleKey: "dashboard.aiAgent.title",
    descKey: "dashboard.aiAgent.desc",
    phaseKey: "dashboard.aiAgent.phase",
    icon: Bot,
  },
];

/**
 * Landing view. Orients the user to what the workspace is and what comes next
 * in the roadmap, without pretending features that don't exist yet are live.
 */
export function DashboardView() {
  const setView = useUIStore((s) => s.setView);
  const { t } = useTranslation();

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("dashboard.title")}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("dashboard.subtitle")}
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {ROADMAP.map(({ view, titleKey, descKey, phaseKey, icon: Icon }) => (
          <Card key={view}>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon className="h-4 w-4 text-primary" />
                  {t(titleKey)}
                </CardTitle>
                <span className="rounded-full border border-border px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
                  {t(phaseKey)}
                </span>
              </div>
              <CardDescription>{t(descKey)}</CardDescription>
            </CardHeader>
            <CardContent>
              <button
                type="button"
                onClick={() => setView(view)}
                className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
              >
                {t("dashboard.openPanel")} <ArrowRight className="h-3 w-3" />
              </button>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
