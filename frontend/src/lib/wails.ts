import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";

/**
 * Shared Wails integration helpers.
 *
 * The generated Service bindings live under `@/../bindings/<module>` and are
 * imported directly where needed; this module covers cross-cutting concerns
 * (events, polling) that don't belong to a single feature.
 */

/**
 * Subscribe to the backend `time` event (emitted every second by main.go) and
 * re-render on each tick. Used by the StatusBar clock.
 *
 * Returns the latest RFC1123 timestamp string, or null until the first event.
 */
export function useClock(): string | null {
  const [time, setTime] = useState<string | null>(null);

  useEffect(() => {
    const cancel = Events.On("time", (event: unknown) => {
      const data = (event as { data?: unknown }).data;
      if (typeof data === "string") {
        setTime(data);
      }
    });
    // Events.On returns a cancel function in wails v3.
    return () => {
      if (typeof cancel === "function") cancel();
    };
  }, []);

  return time;
}
