import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

import { ConfigService } from "@/../bindings/github.com/ai-remote/workspace/internal/interfaces";
import type { AppConfig } from "@/../bindings/github.com/ai-remote/workspace/internal/domain/models";

/** Query key for the app config (shared by settings + terminal). */
export const APP_CONFIG_KEY = ["app-config"] as const;

/**
 * useAppConfig — shared React Query wrapper over ConfigService.GetAppConfig.
 * Settings updates go through updateAppConfig so every consumer (e.g. the
 * terminal picking up font changes live) sees the new value immediately.
 */
export function useAppConfig() {
  return useQuery({
    queryKey: APP_CONFIG_KEY,
    queryFn: () => ConfigService.GetAppConfig(),
    staleTime: Infinity, // changes only through the app itself
  });
}

/** Update the cached config after a successful save. */
export function useSetAppConfig() {
  const queryClient = useQueryClient();
  return useCallback(
    (config: AppConfig) => {
      queryClient.setQueryData(APP_CONFIG_KEY, config);
    },
    [queryClient],
  );
}
