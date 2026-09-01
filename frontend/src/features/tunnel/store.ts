import { create } from "zustand";
import { Events } from "@wailsio/runtime";

import { statusStoreKey, type TunnelStatusDTO } from "./api";

/**
 * Live tunnel status per host RULE, fed by backend "tunnel:status" events
 * (a host can run several tunnels — entries are keyed by hostId + rule key).
 *
 * The subscription is module-level and set up once (subscribeTunnelEvents):
 * statuses keep updating even while the panel is closed, so reopening the
 * panel shows the current state immediately.
 */
interface TunnelStoreState {
  statuses: Record<string, TunnelStatusDTO>;
  apply: (s: TunnelStatusDTO) => void;
}

export const useTunnelStore = create<TunnelStoreState>((set) => ({
  statuses: {},
  apply: (s) =>
    set((prev) => ({
      statuses: { ...prev.statuses, [statusStoreKey(s)]: s },
    })),
}));

let subscribed = false;

/** Idempotently subscribe to backend tunnel status events. */
export function subscribeTunnelEvents() {
  if (subscribed) return;
  subscribed = true;
  Events.On("tunnel:status", (e: unknown) => {
    const data = (e as { data?: unknown }).data as TunnelStatusDTO | undefined;
    if (data?.hostId) {
      useTunnelStore.getState().apply(data);
    }
  });
}
