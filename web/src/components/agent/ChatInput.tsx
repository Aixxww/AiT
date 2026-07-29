import {
  useRef,
  useState,
  useCallback,
  useEffect,
  useImperativeHandle,
  forwardRef,
} from 'react'
import { ArrowUp } from 'lucide-react'
import { Button } from '../ui/Button'

export interface ChatInputHandle {
  focus: () => void
  clear: () => void
  getValue: () => string
}

interface ChatInputProps {
  language: string
  loading: boolean
  onSend: (text: string) => void
}

export const ChatInput = forwardRef<ChatInputHandle, ChatInputProps>(
  function ChatInput({ language, loading, onSend }, ref) {
    const [input, setInput] = useState('')
    const [composing, setComposing] = useState(false)
    const inputRef = useRef<HTMLTextAreaElement>(null)

    useImperativeHandle(ref, () => ({
      focus: () => inputRef.current?.focus(),
      clear: () => {
        setInput('')
        if (inputRef.current) inputRef.current.style.height = 'auto'
      },
      getValue: () => input,
    }))

    const handleInputChange = useCallback(
      (e: React.ChangeEvent<HTMLTextAreaElement>) => {
        setInput(e.target.value)
        const el = e.target
        el.style.height = 'auto'
        el.style.height = Math.min(el.scrollHeight, 150) + 'px'
      },
      []
    )

    const handleSend = () => {
      const msg = input.trim()
      if (!msg || loading) return
      setInput('')
      if (inputRef.current) inputRef.current.style.height = 'auto'
      onSend(msg)
      inputRef.current?.focus()
    }

    // Keyboard shortcut: Cmd+K to focus
    useEffect(() => {
      const handleKeyDown = (e: KeyboardEvent) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
          e.preventDefault()
          inputRef.current?.focus()
        }
      }
      window.addEventListener('keydown', handleKeyDown)
      return () => window.removeEventListener('keydown', handleKeyDown)
    }, [])

    return (
      <div
        style={{
          padding: '12px 16px 20px',
          borderTop: '1px solid var(--color-header-border)',
          background:
            'linear-gradient(to top, var(--color-background) 80%, transparent)',
        }}
      >
        <div
          className="chat-input-wrapper"
          style={{
            maxWidth: 720,
            margin: '0 auto',
            display: 'flex',
            gap: 8,
            background:
              'color-mix(in srgb, var(--color-foreground) 3%, transparent)',
            border: '1px solid var(--color-border)',
            borderRadius: 18,
            padding: '4px 4px 4px 16px',
            alignItems: 'flex-end',
            transition: 'all 0.2s ease',
          }}
        >
          <textarea
            ref={inputRef}
            value={input}
            onChange={handleInputChange}
            onCompositionStart={() => setComposing(true)}
            onCompositionEnd={() => setComposing(false)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey && !composing) {
                e.preventDefault()
                handleSend()
              }
            }}
            placeholder={
              language === 'zh'
                ? '跟 AiTi 聊点什么...  ⌘K'
                : 'Ask AiTi anything...  ⌘K'
            }
            rows={1}
            style={{
              flex: 1,
              background: 'none',
              border: 'none',
              color: 'var(--color-foreground)',
              fontSize: 13.5,
              outline: 'none',
              padding: '10px 0',
              fontFamily: 'inherit',
              resize: 'none',
              lineHeight: 1.5,
              maxHeight: 150,
            }}
          />
          <Button
            variant="unstyled"
            onClick={handleSend}
            disabled={loading || !input.trim()}
            style={{
              width: 36,
              height: 36,
              borderRadius: 12,
              border: 'none',
              background:
                loading || !input.trim()
                  ? 'color-mix(in srgb, var(--color-foreground) 4%, transparent)'
                  : 'linear-gradient(135deg, var(--color-primary), color-mix(in srgb, var(--color-primary) 85%, black))',
              color:
                loading || !input.trim()
                  ? 'var(--color-disabled-fg)'
                  : 'var(--color-primary-fg)',
              cursor: loading || !input.trim() ? 'not-allowed' : 'pointer',
              display: 'grid',
              placeItems: 'center',
              flexShrink: 0,
              transition: 'all 0.2s ease',
            }}
          >
            <ArrowUp size={16} strokeWidth={2.5} />
          </Button>
        </div>
        <div
          style={{
            maxWidth: 720,
            margin: '6px auto 0',
            textAlign: 'center',
            fontSize: 10,
            color: 'var(--color-disabled-fg)',
          }}
        >
          AiTi may make mistakes. Always verify trading decisions.
        </div>
      </div>
    )
  }
)
