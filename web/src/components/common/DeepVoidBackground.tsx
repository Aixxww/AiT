import React from 'react'

interface DeepVoidBackgroundProps extends React.HTMLAttributes<HTMLDivElement> {
  children?: React.ReactNode
  className?: string
}

/**
 * Plain full-height page container. Historically this rendered the cyber
 * grid / scanline decoration layers; the Pro theme renders on the flat
 * token background instead.
 */
export function DeepVoidBackground({
  children,
  className = '',
  ...props
}: DeepVoidBackgroundProps) {
  return (
    <div
      className={`relative flex min-h-screen w-full flex-col overflow-hidden bg-background text-foreground ${className}`}
      {...props}
    >
      <div className="relative z-10 flex h-full w-full flex-1 flex-col">
        {children}
      </div>
    </div>
  )
}
