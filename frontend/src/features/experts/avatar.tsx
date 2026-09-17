import {
  Bot,
  Container,
  Database,
  Network,
  Rocket,
  Stethoscope,
  TerminalSquare,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Expert identity rendering: each ops expert picks a lucide icon name
 * and a named gradient scheme; unknown values degrade to the default bot
 * look so a bad entry can never break the UI.
 */

/** name → icon map (only the icons offered by the expert form). */
const EXPERT_ICONS: Record<string, LucideIcon> = {
  Stethoscope,
  Network,
  Rocket,
  Container,
  TerminalSquare,
  Database,
  Bot,
};

/** Kept as the canonical option list for the expert form (stable order). */
export const EXPERT_ICON_IDS = [
  "Stethoscope",
  "Network",
  "Rocket",
  "Container",
  "TerminalSquare",
  "Database",
  "Bot",
] as const;

/** Avatar gradient stops by color id (static classes — Tailwind JIT needs
 *  literal class names, so dynamic `from-${color}` strings never work). */
export const EXPERT_COLOR_GRADIENTS: Record<string, string> = {
  red: "from-red-500 to-red-500/55",
  blue: "from-blue-500 to-blue-500/55",
  violet: "from-violet-500 to-violet-500/55",
  cyan: "from-cyan-500 to-cyan-500/55",
  green: "from-emerald-500 to-emerald-500/55",
  amber: "from-amber-500 to-amber-500/55",
};

/** Avatar gradient schemes by color id (gradient stops + tinted ring). */
const EXPERT_COLORS: Record<string, string> = {
  red: "from-red-500 to-red-500/55 ring-red-400/30",
  blue: "from-blue-500 to-blue-500/55 ring-blue-400/30",
  violet: "from-violet-500 to-violet-500/55 ring-violet-400/30",
  cyan: "from-cyan-500 to-cyan-500/55 ring-cyan-400/30",
  green: "from-emerald-500 to-emerald-500/55 ring-emerald-400/30",
  amber: "from-amber-500 to-amber-500/55 ring-amber-400/30",
};

export const EXPERT_COLOR_IDS = Object.keys(EXPERT_COLORS);

function expertIcon(name: string | undefined): LucideIcon {
  return (name && EXPERT_ICONS[name]) || Bot;
}

export function expertAvatarClass(color: string | undefined): string {
  return EXPERT_COLORS[color ?? ""] ?? "from-primary to-primary/55 ring-primary/30";
}

/**
 * ExpertAvatar is the round gradient identity badge used in the chat
 * (assistant messages, header pill) and the management list.
 */
export function ExpertAvatar({
  icon,
  color,
  className,
  iconClassName,
}: {
  icon?: string;
  color?: string;
  className?: string;
  iconClassName?: string;
}) {
  const Icon = expertIcon(icon);
  return (
    <div
      aria-hidden
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full bg-gradient-to-br shadow-sm ring-1",
        expertAvatarClass(color),
        className ?? "h-7 w-7",
      )}
    >
      <Icon className={cn("text-white", iconClassName ?? "h-3.5 w-3.5")} />
    </div>
  );
}

/** Small inline icon for selectors/pickers. */
export function ExpertIcon({ icon, className }: { icon?: string; className?: string }) {
  const Icon = expertIcon(icon);
  return <Icon className={cn("h-3.5 w-3.5", className)} />;
}

/** Default-assistant avatar (the general helper, no persona). */
export function GeneralAssistantAvatar({ className, iconClassName }: { className?: string; iconClassName?: string }) {
  return (
    <div
      aria-hidden
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-primary to-primary/55 shadow-sm ring-1 ring-primary/30",
        className ?? "h-7 w-7",
      )}
    >
      <Bot className={cn("text-primary-foreground", iconClassName ?? "h-3.5 w-3.5")} />
    </div>
  );
}
