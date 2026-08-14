import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  providersApi,
  type SaveProviderInput,
} from "@/features/settings/api";

/** Query key for the model-provider list (shared by settings + agent panel). */
export const PROVIDERS_KEY = ["model-providers"] as const;

/** useModelProviders subscribes to the provider list via TanStack Query. */
export function useModelProviders() {
  return useQuery({
    queryKey: PROVIDERS_KEY,
    queryFn: () => providersApi.list(),
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
