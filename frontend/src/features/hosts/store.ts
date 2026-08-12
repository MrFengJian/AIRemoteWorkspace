import { create } from "zustand";

/**
 * Hosts feature state. Phase 1: empty scaffold — Phase 2 populates this with
 * the host list, CRUD status, and connection state from the backend.
 */
interface HostsState {
  /** Populated in Phase 2 once Host CRUD + SSH connection manager land. */
  count: number;
}

export const useHostsStore = create<HostsState>(() => ({
  count: 0,
}));
