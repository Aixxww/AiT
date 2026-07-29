import { motion } from 'framer-motion'
import { ShieldAlert, ArrowLeft, Twitter, Send, Lock } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { OFFICIAL_LINKS } from '../../constants/branding'
import { Button } from '../ui/Button'

interface WhitelistFullPageProps {
  onBack?: () => void
}

export function WhitelistFullPage({ onBack }: WhitelistFullPageProps) {
  const navigate = useNavigate()

  const handleBackToLogin = () => {
    if (onBack) {
      onBack()
    } else {
      navigate('/login')
    }
  }

  return (
    <div className="min-h-screen bg-background text-foreground font-mono relative overflow-hidden flex items-center justify-center px-4">
      {/* Background Grid & Scanlines */}
      <div className="fixed inset-0 bg-[linear-gradient(to_right,color-mix(in_srgb,var(--color-muted-fg)_7%,transparent)_1px,transparent_1px),linear-gradient(to_bottom,color-mix(in_srgb,var(--color-muted-fg)_7%,transparent)_1px,transparent_1px)] bg-[size:24px_24px] pointer-events-none"></div>
      <div className="fixed inset-0 bg-gradient-to-t from-black via-transparent to-transparent pointer-events-none"></div>
      <div className="fixed inset-0 pointer-events-none opacity-[0.03] bg-[linear-gradient(transparent_50%,color-mix(in_srgb,var(--color-background)_70%,transparent)_50%)] bg-[length:100%_4px]"></div>

      <motion.div
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.5 }}
        className="max-w-lg w-full relative z-10"
      >
        <div className="bg-surface/40 backdrop-blur-md border border-loss/30 rounded-lg overflow-hidden relative group">
          {/* Top Bar */}
          <div className="flex items-center justify-between px-4 py-2 bg-loss/15 border-b border-loss/30">
            <div className="flex gap-1.5 opacity-50">
              <div className="w-2.5 h-2.5 rounded-full bg-loss"></div>
              <div className="w-2.5 h-2.5 rounded-full bg-muted"></div>
              <div className="w-2.5 h-2.5 rounded-full bg-muted"></div>
            </div>
            <div className="text-[10px] text-loss font-mono tracking-widest animate-pulse">
              ACCESS_DENIED // ERROR_403
            </div>
          </div>

          <div className="p-8 text-center">
            {/* Icon */}
            <div className="relative mx-auto mb-8 w-20 h-20 flex items-center justify-center">
              <div className="absolute inset-0 bg-loss/20 rounded-full animate-ping opacity-50"></div>
              <div className="relative z-10 p-4 border-2 border-loss/50 rounded-full bg-black/50">
                <ShieldAlert className="w-8 h-8 text-loss" />
              </div>
            </div>

            {/* Title */}
            <h1 className="text-2xl font-bold mb-2 tracking-widest text-foreground uppercase glitch-text">
              <span className="text-loss">RESTRICTED</span> ACCESS
            </h1>

            <div className="h-[1px] w-full bg-gradient-to-r from-transparent via-loss/30 to-transparent my-4"></div>

            {/* Description */}
            <p className="text-xs text-muted-foreground mb-8 leading-relaxed font-mono px-4">
              <span className="text-loss">[SYSTEM_MESSAGE]:</span> YOUR
              IDENTIFIER IS NOT ON THE ACTIVE WHITELIST.
              <br />
              <br />
              Platform capacity limits have been reached for the current beta
              phase. Prioritized access is currently reserved for authorized
              operators only.
            </p>

            {/* Info Box */}
            <div className="bg-loss/10 border border-loss/25 p-4 rounded mb-8 text-left">
              <div className="flex items-start gap-3">
                <Lock className="w-4 h-4 text-loss mt-0.5" />
                <div>
                  <h3 className="text-xs font-bold text-loss uppercase mb-1">
                    Authorization Protocol
                  </h3>
                  <p className="text-[10px] text-muted-foreground leading-tight">
                    Access is rolled out in batches. If you believe this is an
                    error, please verify your credentials or contact system
                    administrators.
                  </p>
                </div>
              </div>
            </div>

            {/* Action Buttons */}
            <div className="space-y-3">
              <Button
                variant="unstyled"
                onClick={handleBackToLogin}
                className="w-full flex items-center justify-center gap-2 py-3 border border-border bg-black hover:bg-surface hover:border-loss hover:text-loss text-muted-foreground transition-all text-xs font-bold tracking-widest uppercase group"
              >
                <ArrowLeft className="w-3 h-3 group-hover:-translate-x-1 transition-transform" />
                RETURN TO LOGIN
              </Button>

              <div className="grid grid-cols-2 gap-3 mt-4">
                <a
                  href={OFFICIAL_LINKS.twitter}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-center gap-2 py-2 border border-border bg-surface/50 hover:bg-surface-alt text-muted-foreground hover:text-foreground transition-colors text-[10px] uppercase"
                >
                  <Twitter className="w-3 h-3" />
                  Updates
                </a>
                <a
                  href={OFFICIAL_LINKS.telegram}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-center gap-2 py-2 border border-border bg-surface/50 hover:bg-surface-alt text-muted-foreground hover:text-foreground transition-colors text-[10px] uppercase"
                >
                  <Send className="w-3 h-3" />
                  Support
                </a>
              </div>
            </div>
          </div>

          {/* Footer */}
          <div className="bg-black/80 p-2 text-[9px] text-muted-foreground/50 text-center border-t border-border font-mono uppercase">
            ERR_CODE: WLIST_0x403 // SECURITY_LAYER_ACTIVE
          </div>
        </div>
      </motion.div>
    </div>
  )
}
