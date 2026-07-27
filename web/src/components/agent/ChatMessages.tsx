import { forwardRef } from 'react'
import { User, Zap } from 'lucide-react'
import { motion } from 'framer-motion'
import { AgentStepPanel } from './AgentStepPanel'
import { renderMessageContent } from './MessageRenderer'

interface AgentStep {
  id: string
  label: string
  status: 'planning' | 'pending' | 'running' | 'completed' | 'replanned'
  detail?: string
}

interface Message {
  id: string
  role: 'user' | 'bot'
  text: string
  time: string
  streaming?: boolean
  steps?: AgentStep[]
}

interface ChatMessagesProps {
  messages: Message[]
}

function hasMeaningfulExecutionSteps(steps?: AgentStep[]) {
  if (!steps || steps.length === 0) return false
  return steps.some((step) => step.status !== 'planning')
}

export const ChatMessages = forwardRef<HTMLDivElement, ChatMessagesProps>(
  function ChatMessages({ messages }, ref) {
    return (
      <div style={{ maxWidth: 720, margin: '0 auto', padding: '0 20px' }}>
        {messages.map((m) => (
          <motion.div
            key={m.id}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.2 }}
            style={{
              display: 'flex',
              gap: 12,
              marginBottom: 24,
              flexDirection: m.role === 'user' ? 'row-reverse' : 'row',
            }}
          >
            {/* Avatar */}
            <div
              style={{
                width: 30,
                height: 30,
                borderRadius: 10,
                display: 'grid',
                placeItems: 'center',
                fontSize: 14,
                flexShrink: 0,
                marginTop: 2,
                background:
                  m.role === 'user'
                    ? 'linear-gradient(135deg, color-mix(in srgb, var(--color-purple) 12%, transparent), color-mix(in srgb, var(--color-purple) 4%, transparent))'
                    : 'linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 8%, transparent), color-mix(in srgb, var(--color-profit) 4%, transparent))',
                border:
                  '1px solid ' +
                  (m.role === 'user'
                    ? 'color-mix(in srgb, var(--color-purple) 15%, transparent)'
                    : 'color-mix(in srgb, var(--color-primary) 10%, transparent)'),
              }}
            >
              {m.role === 'user' ? (
                <User className="w-4 h-4" />
              ) : (
                <Zap className="w-4 h-4" />
              )}
            </div>

            {/* Message content */}
            <div style={{ maxWidth: '78%', minWidth: 0 }}>
              {m.role === 'user' ? (
                <div
                  style={{
                    padding: '10px 16px',
                    borderRadius: 18,
                    borderTopRightRadius: 4,
                    fontSize: 13.5,
                    lineHeight: 1.7,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                    background:
                      'linear-gradient(135deg, var(--color-purple), color-mix(in srgb, var(--color-purple) 80%, black))',
                    color: '#fff',
                  }}
                >
                  {m.text}
                </div>
              ) : (
                <div
                  style={{
                    padding: '12px 16px',
                    borderRadius: 18,
                    borderTopLeftRadius: 4,
                    fontSize: 13.5,
                    lineHeight: 1.7,
                    wordBreak: 'break-word',
                    background: 'rgba(255,255,255,0.03)',
                    color: 'var(--color-foreground)',
                    border: '1px solid rgba(255,255,255,0.05)',
                  }}
                >
                  <AgentStepPanel
                    steps={m.steps}
                    visible={hasMeaningfulExecutionSteps(m.steps)}
                  />
                  {renderMessageContent(m.text)}
                  {m.streaming && m.text === '' && (
                    <div style={{ display: 'flex', gap: 4, padding: '4px 0' }}>
                      <span
                        className="typing-dot"
                        style={{ animationDelay: '0ms' }}
                      />
                      <span
                        className="typing-dot"
                        style={{ animationDelay: '150ms' }}
                      />
                      <span
                        className="typing-dot"
                        style={{ animationDelay: '300ms' }}
                      />
                    </div>
                  )}
                  {m.streaming && m.text !== '' && (
                    <span
                      style={{
                        display: 'inline-block',
                        width: 2,
                        height: 15,
                        background: 'var(--color-primary)',
                        marginLeft: 1,
                        borderRadius: 1,
                        animation: 'blink 0.8s infinite',
                        verticalAlign: 'text-bottom',
                      }}
                    />
                  )}
                </div>
              )}
              {m.time && !m.streaming && (
                <div
                  style={{
                    fontSize: 10,
                    color: 'var(--color-disabled-fg)',
                    marginTop: 4,
                    textAlign: m.role === 'user' ? 'right' : 'left',
                    paddingLeft: m.role === 'bot' ? 4 : 0,
                    paddingRight: m.role === 'user' ? 4 : 0,
                  }}
                >
                  {m.role === 'bot' && 'AiTi · '}
                  {m.time}
                </div>
              )}
            </div>
          </motion.div>
        ))}
        <div ref={ref} />
      </div>
    )
  }
)
