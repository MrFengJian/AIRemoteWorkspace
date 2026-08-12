import { Bot } from "lucide-react";

import { PlaceholderView } from "@/components/layout/PlaceholderView";

/**
 * AI Agent panel. Phase 1: placeholder.
 * Phase 4 will add the LLM provider, agent runtime, tool registry, and
 * permission-gated tool execution (AGENT.md §12-14).
 */
export function AgentView() {
  return (
    <PlaceholderView
      icon={Bot}
      title="AI Agent"
      description="LLM-driven operations through a permission-gated tool runtime. Ask the agent to diagnose a host; it connects, inspects, and proposes fixes."
      phase="Phase 4 — AI Agent"
    />
  );
}
