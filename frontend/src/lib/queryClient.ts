import { QueryClient } from "@tanstack/react-query";

/**
 * Shared TanStack Query client.
 * Defaults favour a desktop-app feel: short stale time, no refetch on focus
 * (the window already owns the data lifecycle).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});
