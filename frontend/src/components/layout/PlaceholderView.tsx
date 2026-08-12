import type { LucideIcon } from "lucide-react";

interface PlaceholderViewProps {
  icon: LucideIcon;
  title: string;
  description: string;
  phase: string;
}

/**
 * Placeholder shown for feature areas that are scaffolded but not yet
 * implemented. Keeps the navigation honest about what exists today vs. what
 * the roadmap promises.
 */
export function PlaceholderView({
  icon: Icon,
  title,
  description,
  phase,
}: PlaceholderViewProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
      <Icon className="h-10 w-10 text-muted-foreground" />
      <div>
        <h2 className="text-lg font-medium text-foreground">{title}</h2>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      </div>
      <span className="rounded-full border border-border px-3 py-1 text-[10px] uppercase tracking-wide text-muted-foreground">
        {phase}
      </span>
    </div>
  );
}
