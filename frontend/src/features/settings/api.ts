// UI-only helpers for the Settings feature.
//
// Domain types (AppConfig, SecurityMode) come from the generated Wails
// bindings — do not redeclare them here. This file holds pure presentation
// helpers that have no backend analogue.

/**
 * Human-readable labels for each security mode value.
 * Keys are the string values of the generated SecurityMode enum, so they
 * stay decoupled from the enum's member names.
 */
export const SECURITY_MODE_LABELS: Record<string, string> = {
  convenience: "Convenience",
  balanced: "Balanced (default)",
  secure: "Secure",
};
