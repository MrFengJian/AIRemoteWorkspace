/**
 * TerminalTabMenu is now a thin alias of the shared ContextMenu component
 * (same MenuItem contract) — kept so existing terminal imports stay stable.
 */
export { ContextMenu as TerminalTabMenu } from "@/components/ui/ContextMenu";
export type { MenuItem } from "@/components/ui/ContextMenu";
