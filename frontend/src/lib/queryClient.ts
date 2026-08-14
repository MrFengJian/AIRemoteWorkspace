import { MutationCache, QueryClient } from "@tanstack/react-query";

import { toast } from "@/lib/toast";

/**
 * Shared TanStack Query client.
 * Defaults favour a desktop-app feel: short stale time, no refetch on focus
 * (the window already owns the data lifecycle).
 *
 * A global MutationCache onError surfaces any unhandled mutation failure as
 * a toast, so no call site can fail silently.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      // Mutations that manage their own error UI set meta.silent to opt out.
      if (mutation.meta?.silent) return;
      toast.error(error instanceof Error ? error.message : String(error));
    },
  }),
});
