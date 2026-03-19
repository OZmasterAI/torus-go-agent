const BASE = process.env["TORUS_URL"] ?? "http://localhost:8080"

export interface StatusInfo {
  provider: string
  model: string
  branch: string
  head: string
  messages: number
}

export interface SSEEvent {
  type: "start" | "delta" | "done" | "error"
  text?: string
  error?: string
}

export async function fetchStatus(): Promise<StatusInfo> {
  const res = await fetch(`${BASE}/api/status`)
  return res.json() as Promise<StatusInfo>
}

export async function createSession(): Promise<{ branch: string }> {
  const res = await fetch(`${BASE}/api/new`, { method: "POST" })
  return res.json() as Promise<{ branch: string }>
}

export async function clearContext(): Promise<void> {
  await fetch(`${BASE}/api/clear`, { method: "POST" })
}

export async function sendMessage(
  text: string,
  onStart: () => void,
  onDelta: (text: string) => void,
  onDone: (fullText: string) => void,
  onError: (error: string) => void,
): Promise<void> {
  const res = await fetch(`${BASE}/api/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message: text }),
  })

  if (!res.ok) {
    onError(`HTTP ${res.status}`)
    return
  }

  const reader = res.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const parts = buffer.split("\n\n")
    buffer = parts.pop()!

    for (const part of parts) {
      for (const line of part.split("\n")) {
        if (!line.startsWith("data: ")) continue
        try {
          const data = JSON.parse(line.slice(6)) as SSEEvent
          switch (data.type) {
            case "start":
              onStart()
              break
            case "delta":
              onDelta(data.text ?? "")
              break
            case "done":
              onDone(data.text ?? "")
              break
            case "error":
              onError(data.error ?? "Unknown error")
              break
          }
        } catch {
          // ignore malformed SSE lines
        }
      }
    }
  }
}
