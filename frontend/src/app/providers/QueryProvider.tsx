import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";

import { queryClient } from "@/lib/queryClient";

/**
 * Wraps the app with TanStack Query. Kept as its own provider so the
 * provider tree stays explicit (theme, confirm, toast) without touching
 * main.tsx.
 */
export function QueryProvider({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}
