/**
 * UTF-8-safe base64 helpers.
 *
 * The backend ships binary payloads (PTY output, agent chunks) and accepts
 * terminal input as base64. Native atob/btoa are Latin-1 only: atob returns a
 * binary string (multi-byte UTF-8 turns into mojibake) and btoa throws on any
 * character above U+00FF (Chinese input/insert would silently fail). Always
 * route through these helpers instead.
 */

/** base64 → raw bytes. */
export function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

/** base64 → UTF-8 string. Only use when chunks are known to be complete UTF-8
 *  (e.g. JSON-decoded LLM deltas); for arbitrary byte streams (PTY output)
 *  prefer base64ToBytes + a streaming decoder like xterm's write(). */
export function decodeBase64(b64: string): string {
  return new TextDecoder().decode(base64ToBytes(b64));
}

/** UTF-8 string → base64. */
export function encodeBase64(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let bin = "";
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin);
}

/** raw bytes → base64 (binary-safe, e.g. clipboard images). */
export function bytesToBase64(bytes: Uint8Array): string {
  let bin = "";
  const chunk = 0x8000; // avoid arg-length limits on large buffers
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}
