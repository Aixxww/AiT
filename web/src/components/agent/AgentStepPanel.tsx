interface AgentStep {
  id: string
  label: string
  status: 'planning' | 'pending' | 'running' | 'completed' | 'replanned'
  detail?: string
}

interface AgentStepPanelProps {
  steps?: AgentStep[]
  visible?: boolean
}

const statusStyles: Record<AgentStep['status'], { dot: string; text: string }> =
  {
    planning: { dot: 'var(--color-purple)', text: 'var(--color-purple)' },
    pending: {
      dot: 'color-mix(in srgb, var(--color-muted-fg) 30%, transparent)',
      text: 'var(--color-muted-fg)',
    },
    running: { dot: 'var(--color-primary)', text: 'var(--color-primary)' },
    completed: { dot: 'var(--color-profit)', text: 'var(--color-profit)' },
    replanned: { dot: 'var(--color-info)', text: 'var(--color-info)' },
  }

export function AgentStepPanel({ steps, visible }: AgentStepPanelProps) {
  if (!visible || !steps || steps.length === 0) {
    return null
  }

  return (
    <div
      style={{
        marginBottom: 12,
        padding: '10px 12px',
        borderRadius: 12,
        background:
          'linear-gradient(180deg, color-mix(in srgb, var(--color-foreground) 3%, transparent), color-mix(in srgb, var(--color-foreground) 2%, transparent))',
        border: '1px solid var(--color-border)',
      }}
    >
      <div
        style={{
          fontSize: 11,
          fontWeight: 700,
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--color-muted-fg)',
          marginBottom: 10,
        }}
      >
        Live Run
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {steps.map((step) => {
          const style = statusStyles[step.status]
          return (
            <div
              key={step.id}
              style={{
                display: 'grid',
                gridTemplateColumns: '14px 1fr',
                gap: 8,
                alignItems: 'start',
              }}
            >
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: 999,
                  marginTop: 5,
                  background: style.dot,
                  boxShadow:
                    step.status === 'running'
                      ? '0 0 0 4px color-mix(in srgb, var(--color-primary) 8%, transparent)'
                      : 'none',
                }}
              />
              <div>
                <div
                  style={{
                    fontSize: 12.5,
                    lineHeight: 1.5,
                    color: style.text,
                    fontWeight: step.status === 'running' ? 600 : 500,
                  }}
                >
                  {step.label}
                </div>
                {step.detail && (
                  <div
                    style={{
                      fontSize: 11.5,
                      lineHeight: 1.45,
                      color: 'var(--color-muted-fg)',
                      marginTop: 2,
                    }}
                  >
                    {step.detail}
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
