const RETRY_DELAY = 100;
const RETRY_TIMEOUT = 5000;

export async function withRetry<T>(fn: () => Promise<T>): Promise<T> {
  const deadline = Date.now() + RETRY_TIMEOUT;

  // First attempt
  try {
    return await fn();
  } catch (error) {
    // Fall through to retry loop
  }

  // Retry loop
  while (Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY));
    try {
      return await fn();
    } catch (error) {
      // Continue retrying
    }
  }

  // Final attempt (will throw if it fails)
  return await fn();
}
