/**
 * Shared utility functions for ParamView and SecretView
 */

/**
 * Mask sensitive values for display
 */
export function maskValue(value: string): string {
  return '*'.repeat(Math.min(value.length, 32));
}

/**
 * Format ISO date string to locale string
 */
export function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString();
}

/**
 * Format JSON value with pretty printing, or return as-is if not valid JSON
 */
export function formatJsonValue(value: string): string {
  try {
    const parsed = JSON.parse(value);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return value;
  }
}

/**
 * Get operation color class for staging view
 */
export function getOperationColor(operation: string): string {
  switch (operation) {
    case 'create': return 'op-create';
    case 'update': return 'op-update';
    case 'delete': return 'op-delete';
    default: return '';
  }
}

/**
 * Create a debounced function
 */
export function createDebouncer(delayMs: number = 300) {
  let timer: ReturnType<typeof setTimeout> | null = null;

  return (callback: () => void) => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(callback, delayMs);
  };
}

/**
 * Parse error to string message
 */
export function parseError(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
