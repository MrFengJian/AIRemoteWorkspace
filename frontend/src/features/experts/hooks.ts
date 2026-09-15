import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { expertsApi } from "@/features/experts/api";

/** Query key for the expert roster (shared by settings + agent panel). */
export const EXPERTS_KEY = ["experts"] as const;

/** useExperts subscribes to the digital-employee roster. Local SQLite reads
 *  are cheap; refetchOnMount keeps the pickers in sync with edits made in
 *  the settings page without any event wiring. */
export function useExperts() {
  return useQuery({
    queryKey: EXPERTS_KEY,
    queryFn: expertsApi.list,
    retry: 2,
    refetchOnMount: "always",
  });
}

/** useSaveExpert mutates and refreshes the shared roster. */
export function useSaveExpert() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: expertsApi.save,
    onSuccess: () => qc.invalidateQueries({ queryKey: EXPERTS_KEY }),
  });
}

/** useDeleteExpert mutates and refreshes the shared roster. */
export function useDeleteExpert() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: expertsApi.remove,
    onSuccess: () => qc.invalidateQueries({ queryKey: EXPERTS_KEY }),
  });
}
