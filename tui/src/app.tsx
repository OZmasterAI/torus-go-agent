import { render, useKeyboard, useTerminalDimensions } from "@opentui/solid"
import { createSignal, createEffect, For, Show, onMount } from "solid-js"
import { TextAttributes } from "@opentui/core"
import { theme, createSyntax } from "./theme"
import * as api from "./api"

// The reconciler supports fg/bg on span style at runtime, but the TS types
// declare SpanProps.style as Partial<{}> which doesn't expose fg/bg.
// This helper casts to the runtime-supported shape.
type SpanColorStyle = { fg?: string; bg?: string }
function spanStyle(s: SpanColorStyle): Record<string, unknown> {
  return s as Record<string, unknown>
}

interface Message {
  role: "user" | "assistant" | "error" | "system"
  content: string
}

function App() {
  const dims = useTerminalDimensions()
  const syntax = createSyntax()

  // State
  const [messages, setMessages] = createSignal<Message[]>([])
  const [inputVal, setInputVal] = createSignal("")
  const [streaming, setStreaming] = createSignal(false)
  const [status, setStatus] = createSignal<api.StatusInfo | null>(null)
  const [showSidebar, setShowSidebar] = createSignal(false)

  // Fetch status on mount
  onMount(async () => {
    try {
      const s = await api.fetchStatus()
      setStatus(s)
    } catch {
      setMessages([{ role: "error", content: "Cannot connect to server. Is the agent running with --http?" }])
    }
  })

  createEffect(() => {
    setShowSidebar(dims().width > 120)
  })

  const sidebarWidth = 36

  // Send message
  async function send() {
    const text = inputVal().trim()
    if (!text || streaming()) return

    // Handle slash commands
    if (text === "/new") {
      try {
        await api.createSession()
        setMessages([])
        const s = await api.fetchStatus()
        setStatus(s)
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e)
        setMessages(prev => [...prev, { role: "error", content: msg }])
      }
      setInputVal("")
      return
    }
    if (text === "/clear") {
      try {
        await api.clearContext()
        setMessages([])
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e)
        setMessages(prev => [...prev, { role: "error", content: msg }])
      }
      setInputVal("")
      return
    }
    if (text === "/quit" || text === "/exit") {
      process.exit(0)
    }

    setInputVal("")
    setMessages(prev => [...prev, { role: "user", content: text }])
    setStreaming(true)

    try {
      await api.sendMessage(
        text,
        () => {
          setMessages(prev => [...prev, { role: "assistant", content: "" }])
        },
        (delta) => {
          setMessages(prev => {
            const msgs = [...prev]
            const last = msgs[msgs.length - 1]
            if (last?.role === "assistant") {
              msgs[msgs.length - 1] = { ...last, content: last.content + delta }
            }
            return msgs
          })
        },
        (fullText) => {
          setMessages(prev => {
            const msgs = [...prev]
            const last = msgs[msgs.length - 1]
            if (last?.role === "assistant") {
              msgs[msgs.length - 1] = { ...last, content: fullText }
            }
            return msgs
          })
          setStreaming(false)
        },
        (error) => {
          setMessages(prev => [...prev, { role: "error", content: error }])
          setStreaming(false)
        },
      )
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setMessages(prev => [...prev, { role: "error", content: msg }])
      setStreaming(false)
    }
  }

  // Keyboard handling
  useKeyboard((evt) => {
    if (evt.ctrl && evt.name === "c") {
      if (!streaming()) process.exit(0)
      return
    }
    if (evt.ctrl && evt.name === "d") {
      process.exit(0)
    }
  })

  return (
    <box
      width={dims().width}
      height={dims().height}
      backgroundColor={theme.background}
      flexDirection="column"
    >
      {/* Header */}
      <box
        height={1}
        backgroundColor={theme.backgroundPanel}
        paddingLeft={2}
        paddingRight={2}
        flexDirection="row"
        flexShrink={0}
      >
        <text fg={theme.primary} attributes={TextAttributes.BOLD}>
          <span>Torus</span>
        </text>
        <box flexGrow={1} />
        <Show when={status()}>
          <text fg={theme.textMuted}>
            <span>{status()!.model}</span>
            <span> · </span>
            <span>{status()!.branch.slice(0, 12)}</span>
          </text>
        </Show>
      </box>

      {/* Main area */}
      <box flexDirection="row" flexGrow={1}>
        {/* Chat column */}
        <box flexGrow={1} flexDirection="column" paddingLeft={2} paddingRight={2} paddingTop={1}>
          <scrollbox
            flexGrow={1}
            stickyScroll={true}
            stickyStart="bottom"
            verticalScrollbarOptions={{
              paddingLeft: 1,
              visible: true,
              trackOptions: {
                backgroundColor: theme.backgroundElement,
                foregroundColor: theme.border,
              },
            }}
          >
            <Show when={messages().length === 0}>
              <box flexDirection="column" alignItems="center" paddingTop={4} gap={1}>
                <text fg={theme.primary} attributes={TextAttributes.BOLD}>
                  <span>Torus Agent</span>
                </text>
                <text fg={theme.textMuted}>
                  <span>Ask anything. Enter to send, Ctrl+D to quit.</span>
                </text>
                <text fg={theme.textMuted}>
                  <span>/new  /clear  /exit</span>
                </text>
              </box>
            </Show>
            <For each={messages()}>
              {(msg, index) => (
                <>
                  {/* User message */}
                  <Show when={msg.role === "user"}>
                    <box
                      marginTop={index() > 0 ? 1 : 0}
                      border={["left"]}
                      borderColor={theme.primary}
                    >
                      <box
                        paddingTop={1}
                        paddingBottom={1}
                        paddingLeft={2}
                        backgroundColor={theme.backgroundPanel}
                      >
                        <text fg={theme.text}>
                          <span>{msg.content}</span>
                        </text>
                      </box>
                    </box>
                  </Show>

                  {/* Assistant message */}
                  <Show when={msg.role === "assistant"}>
                    <box paddingLeft={3} marginTop={1} flexShrink={0}>
                      <Show
                        when={msg.content}
                        fallback={
                          <text fg={theme.textMuted}>
                            <span>thinking...</span>
                          </text>
                        }
                      >
                        <code
                          filetype="markdown"
                          drawUnstyledText={false}
                          streaming={streaming() && index() === messages().length - 1}
                          syntaxStyle={syntax}
                          content={msg.content}
                          conceal={true}
                          fg={theme.text}
                        />
                      </Show>
                    </box>
                  </Show>

                  {/* Error message */}
                  <Show when={msg.role === "error"}>
                    <box
                      marginTop={1}
                      border={["left"]}
                      borderColor={theme.error}
                    >
                      <box
                        paddingTop={1}
                        paddingBottom={1}
                        paddingLeft={2}
                        backgroundColor={theme.backgroundPanel}
                      >
                        <text fg={theme.error}>
                          <span>{msg.content}</span>
                        </text>
                      </box>
                    </box>
                  </Show>

                  {/* System message */}
                  <Show when={msg.role === "system"}>
                    <box paddingLeft={3} marginTop={1}>
                      <text fg={theme.textMuted}>
                        <span>{msg.content}</span>
                      </text>
                    </box>
                  </Show>
                </>
              )}
            </For>
          </scrollbox>

          {/* Input area */}
          <box flexShrink={0} marginTop={1} marginBottom={1}>
            <Show
              when={!streaming()}
              fallback={
                <box
                  height={3}
                  border={true}
                  borderColor={theme.borderSubtle}
                  backgroundColor={theme.backgroundPanel}
                  paddingLeft={1}
                  alignItems="center"
                >
                  <text fg={theme.textMuted}>
                    <span>
                      {messages().at(-1)?.content ? "streaming..." : "thinking..."}
                    </span>
                  </text>
                </box>
              }
            >
              <box
                border={true}
                borderColor={theme.borderSubtle}
                backgroundColor={theme.backgroundPanel}
                focusedBorderColor={theme.primary}
                focused={true}
              >
                <input
                  backgroundColor={theme.backgroundPanel}
                  focusedBackgroundColor={theme.backgroundPanel}
                  textColor={theme.text}
                  focusedTextColor={theme.text}
                  placeholder="Send a message... (Enter to send, /new /clear /exit)"
                  placeholderColor={theme.textMuted}
                  value={inputVal()}
                  focused={true}
                  onChange={(val) => { setInputVal(val) }}
                  onSubmit={() => { void send() }}
                />
              </box>
            </Show>
          </box>
        </box>

        {/* Sidebar */}
        <Show when={showSidebar()}>
          <box
            width={sidebarWidth}
            flexShrink={0}
            borderColor={theme.borderSubtle}
            border={["left"]}
            paddingLeft={2}
            paddingTop={1}
            flexDirection="column"
            gap={1}
          >
            <text fg={theme.primary} attributes={TextAttributes.BOLD}>
              <span>Session</span>
            </text>
            <Show when={status()}>
              <text fg={theme.textMuted}>
                <span>Provider  </span>
                <span style={spanStyle({ fg: theme.text })}>{status()!.provider}</span>
              </text>
              <text fg={theme.textMuted}>
                <span>Model     </span>
                <span style={spanStyle({ fg: theme.text })}>{status()!.model}</span>
              </text>
              <text fg={theme.textMuted}>
                <span>Messages  </span>
                <span style={spanStyle({ fg: theme.text })}>{status()!.messages.toString()}</span>
              </text>
              <text fg={theme.textMuted}>
                <span>Branch    </span>
                <span style={spanStyle({ fg: theme.text })}>{status()!.branch.slice(0, 16)}</span>
              </text>
            </Show>
            <box marginTop={1} flexDirection="column" gap={1}>
              <text fg={theme.primary} attributes={TextAttributes.BOLD}>
                <span>Keys</span>
              </text>
              <text fg={theme.textMuted}>
                <span>Enter       </span>
                <span style={spanStyle({ fg: theme.text })}>send</span>
              </text>
              <text fg={theme.textMuted}>
                <span>/new        </span>
                <span style={spanStyle({ fg: theme.text })}>new session</span>
              </text>
              <text fg={theme.textMuted}>
                <span>/clear      </span>
                <span style={spanStyle({ fg: theme.text })}>clear context</span>
              </text>
              <text fg={theme.textMuted}>
                <span>Ctrl+D      </span>
                <span style={spanStyle({ fg: theme.text })}>quit</span>
              </text>
            </box>
          </box>
        </Show>
      </box>

      {/* Status bar */}
      <box
        height={1}
        backgroundColor={theme.backgroundPanel}
        paddingLeft={2}
        paddingRight={2}
        flexShrink={0}
        flexDirection="row"
      >
        <Show when={status()}>
          <text fg={theme.textMuted}>
            <span>{status()!.provider}</span>
            <span>/</span>
            <span>{status()!.model}</span>
            <span> · </span>
            <span>{status()!.messages.toString()}</span>
            <span> messages</span>
          </text>
        </Show>
        <box flexGrow={1} />
        <text fg={theme.textMuted}>
          <span>Torus Agent</span>
        </text>
      </box>
    </box>
  )
}

// Entry point
render(
  () => <App />,
  {
    targetFps: 60,
    exitOnCtrlC: false,
    useKittyKeyboard: {},
    autoFocus: false,
  },
)
