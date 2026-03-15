export type PollOptions = {
  timeoutMs?: number
  intervalMs?: number
}

export async function sleep(ms: number) {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })
}

export async function poll<T>(
  fn: () => Promise<T>,
  isDone: (value: T) => boolean,
  options: PollOptions = {},
): Promise<T> {
  const timeoutMs = options.timeoutMs ?? 5 * 60 * 1000
  const intervalMs = options.intervalMs ?? 3000
  const deadline = Date.now() + timeoutMs
  let lastValue: T | null = null
  let lastError: unknown = null

  while (Date.now() <= deadline) {
    try {
      const value = await fn()
      lastValue = value
      if (isDone(value)) {
        return value
      }
    } catch (error) {
      lastError = error
    }
    await sleep(intervalMs)
  }

  const tail =
    lastError != null ? `lastError=${String(lastError)}` : `lastValue=${JSON.stringify(lastValue)}`
  throw new Error(`poll timeout after ${timeoutMs}ms (${tail})`)
}
