import { Server } from "lucide-react";

import { PlaceholderView } from "@/components/layout/PlaceholderView";

/**
 * Host management view. Phase 1: placeholder.
 * Phase 2 will add the host list, add/edit dialog, and connection testing.
 */
export function HostsView() {
  return (
    <PlaceholderView
      icon={Server}
      title="Host Management"
      description="Add, edit, test, and organise remote machines. Saved locally with sensitive credentials stored in the OS keychain."
      phase="Phase 2 — SSH Workspace"
    />
  );
}
