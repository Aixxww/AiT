import React from 'react'

interface DeepVoidBackgroundProps extends React.HTMLAttributes<HTMLDivElement> {
  children?: React.ReactNode
  className?: string
  disableAnimation?: boolean
}

export function DeepVoidBackground({
  children,
  className = '',
  disableAnimation = false,
  ...props
}: DeepVoidBackgroundProps) {
  return (
    <div
      className={`cyber-page-shell relative flex min-h-screen w-full flex-col overflow-hidden text-ait-text ${className}`}
      {...props}
    >
      <div className="cyber-page-surface absolute inset-0 pointer-events-none z-0" />
      <div className="cyber-page-grid absolute inset-0 pointer-events-none z-0" />
      <div
        className={`cyber-page-scan absolute inset-0 pointer-events-none z-0 ${
          disableAnimation ? '' : 'animate-scan'
        }`}
      />

      {/* Content Layer */}
      <div className="relative z-10 flex h-full w-full flex-1 flex-col">
        {children}
      </div>
    </div>
  )
}
