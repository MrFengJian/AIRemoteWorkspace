import {
  Bot,
  FolderTree,
  Server,
  TerminalSquare,
  ArrowRight,
  type LucideIcon,
} from "lucide-react";

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
  title: string;
  description: string;
  phase: string;
  icon: LucideIcon;
}

const ROADMAP: RoadmapEntry[] = [
  {
    view: "hosts",
    title: "Host Management",
    description: "Add, edit, test, and organise remote machines.",
    phase: "Phase 2",
    icon: Server,
  },
  {
    view: "terminal",
    title: "SSH Terminal",
    description: "Stable PTY-backed terminal over SSH (xterm.js).",
    phase: "Phase 2",
    icon: TerminalSquare,
  },
  {
    view: "sftp",
    title: "File Workspace",
    description: "Browse, upload, download, and edit remote files.",
    phase: "Phase 3",
    icon: FolderTree,
  },
  {
    view: "agent",
    title: "AI Agent",
    description: "LLM + Tool Calling with permission-gated execution.",
    phase: "Phase 4",
    icon: Bot,
  },
];

/**
 * Landing view. Orients the user to what the workspace is and what comes next
 * in the roadmap, without pretending features that don't exist yet are live.
 */
export function DashboardView() {
  const setView = useUIStore((s) => s.setView);

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          AI Remote Workspace
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          A lightweight, local-first, AI-native desktop remote workspace.
          Connect machines, run terminals, manage files, and let an AI agent
          diagnose your infrastructure — all from a single binary.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {ROADMAP.map(({ view, title, description, phase, icon: Icon }) => (
          <Card key={view}>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon className="h-4 w-4 text-primary" />
                  {title}
                </CardTitle>
                <span className="rounded-full border border-border px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
                  {phase}
                </span>
              </div>
              <CardDescription>{description}</CardDescription>
            </CardHeader>
            <CardContent>
              <button
                type="button"
                onClick={() => setView(view)}
                className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
              >
                Open panel <ArrowRight className="h-3 w-3" />
              </button>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
