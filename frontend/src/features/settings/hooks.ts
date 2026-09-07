import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  providersApi,
  type SaveProviderInput,
} from "@/features/settings/api";

/** Query key for the model-provider list (shared by settings + agent panel). */
export const PROVIDERS_KEY = ["model-providers"] as const;

/** Deadline for one ListProviders call. The settings view is mounted (hidden)
 *  from app startup, so this query fires before the Wails bridge handshake
 *  settles — the bridge occasionally drops that call and the promise then
 *  never settles either, which wedged the agent panel's model selector with
 *  no recovery path (TanStack retry only reacts to rejections, and a later
 *  observer mount just subscribes to the in-flight fetch instead of starting
 *  a new one). A deadline turns the silent hang into a retryable rejection. */
const LIST_TIMEOUT_MS = 10_000;

function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("request timed out")), ms);
    p.then(
      (v) => {
        clearTimeout(timer);
        resolve(v);
      },
      (e) => {
        clearTimeout(timer);
        reject(e);
      },
    );
  });
}

/** useModelProviders subscribes to the provider list via TanStack Query.
 *  The list is tiny and read from local SQLite, so observers always re-sync
 *  on mount (refetchOnMount: "always"): opening the agent panel heals a
 *  stale, failed, or pre-config empty startup fetch instead of locking the
 *  model selector until restart. Retries ride out the startup-ready window. */
export function useModelProviders() {
  return useQuery({
    queryKey: PROVIDERS_KEY,
    queryFn: () => withTimeout(providersApi.list(), LIST_TIMEOUT_MS),
    retry: 3,
    retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000),
    refetchOnMount: "always",
  });
}

/** useSaveProvider mutates and refreshes the shared list. */
export function useSaveProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SaveProviderInput) => providersApi.save(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: PROVIDERS_KEY }),
  });
}

/** useDeleteProvider mutates and refreshes the shared list. */
export function useDeleteProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => providersApi.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: PROVIDERS_KEY }),
  });
}
