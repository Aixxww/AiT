import { useState } from 'react'
import { useLanguage } from '../contexts/LanguageContext'
import { t } from '../i18n/translations'
import { BarChart3, TrendingUp, ExternalLink } from 'lucide-react'

type MarketWidget = 'chart' | 'heatmap' | 'ticker'

export function DataPage() {
  const { language } = useLanguage()
  const [activeWidget, setActiveWidget] = useState<MarketWidget>('chart')

  return (
    <div className="w-full min-h-[calc(100vh-64px)] flex flex-col">
      {/* Header */}
      <div className="px-4 sm:px-6 py-4 border-b border-white/5 flex items-center gap-4 flex-wrap">
        <h2 className="text-lg font-bold text-ait-text-main">
          {t('dataCenter', language)}
        </h2>
        <div className="flex items-center gap-2">
          {([
            { key: 'chart', icon: BarChart3, label: language === 'zh' ? '行情图表' : 'Chart' },
            { key: 'heatmap', icon: TrendingUp, label: language === 'zh' ? '热力图' : 'Heatmap' },
          ] as { key: MarketWidget; icon: any; label: string }[]).map((item) => {
            const Icon = item.icon
            return (
              <button
                key={item.key}
                onClick={() => setActiveWidget(item.key)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all border ${activeWidget === item.key
                  ? 'bg-ait-gold/10 text-ait-gold border-ait-gold/20'
                  : 'text-ait-text-muted border-transparent hover:text-white hover:bg-white/5'
                  }`}
              >
                <Icon className="w-3.5 h-3.5 inline mr-1.5" />
                {item.label}
              </button>
            )
          })}
        </div>
      </div>

      {/* Widget Content */}
      <div className="flex-1 relative">
        {activeWidget === 'chart' && (
          <iframe
            src="https://s.tradingview.com/widgetembed/?frameElementId=tradingview_ait&symbol=BINANCE%3ABTCUSDT&interval=D&hidesidetoolbar=0&symboledit=1&saveimage=1&toolbarbg=f1f3f6&studies=[]&theme=dark&style=1&timezone=Etc%2FUTC&withdateranges=1&showpopupbutton=1&studies_overrides={}&overrides={}&enabled_features=[]&disabled_features=[]&locale=en&utm_source=localhost&utm_medium=widget&utm_campaign=chart&utm_term=BINANCE%3ABTCUSDT"
            title={t('dataCenter', language)}
            className="w-full h-[calc(100vh-128px)] border-0"
            allow="fullscreen"
          />
        )}
        {activeWidget === 'heatmap' && (
          <iframe
            src="https://s.tradingview.com/widgetembed/?frameElementId=tradingview_heatmap&symbol=CRYPTO%3AMCAP&interval=1D&hidesidetoolbar=1&theme=dark&style=1&locale=en&utm_source=localhost&utm_medium=widget&utm_campaign=heatmap&utm_term=CRYPTO%3AMCAP"
            title="Crypto Heatmap"
            className="w-full h-[calc(100vh-128px)] border-0"
            allow="fullscreen"
          />
        )}
      </div>

      {/* Footer bar */}
      <div className="px-6 py-2 border-t border-white/5 flex items-center justify-between">
        <span className="text-[10px] text-ait-text-muted/50">
          {language === 'zh' ? '数据来自 TradingView（免费嵌入）' : 'Data via TradingView (free embed)'}
        </span>
        <a
          href="https://www.tradingview.com/"
          target="_blank"
          rel="noopener noreferrer"
          className="text-[10px] text-ait-text-muted/50 hover:text-ait-gold flex items-center gap-1 transition-colors"
        >
          TradingView <ExternalLink className="w-2.5 h-2.5" />
        </a>
      </div>
    </div>
  )
}