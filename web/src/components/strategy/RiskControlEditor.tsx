import { Shield, AlertTriangle } from 'lucide-react'
import type { RiskControlConfig } from '../../types'
import { riskControl, ts } from '../../i18n/strategy-translations'

interface RiskControlEditorProps {
  config: RiskControlConfig
  onChange: (config: RiskControlConfig) => void
  disabled?: boolean
  language: string
}

export function RiskControlEditor({
  config,
  onChange,
  disabled,
  language,
}: RiskControlEditorProps) {
  const updateField = <K extends keyof RiskControlConfig>(
    key: K,
    value: RiskControlConfig[K]
  ) => {
    if (!disabled) {
      onChange({ ...config, [key]: value })
    }
  }

  return (
    <div className="space-y-6">
      {/* Position Limits */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(riskControl.positionLimits, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 gap-4 mb-4">
          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.maxPositions, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.maxPositionsDesc, language)}
            </p>
            <input
              type="number"
              name="risk-max-positions"
              autoComplete="off"
              inputMode="numeric"
              value={config.max_positions ?? 3}
              onChange={(e) =>
                updateField('max_positions', parseInt(e.target.value) || 3)
              }
              disabled={disabled}
              min={1}
              max={3}
              className="w-32 px-3 py-2 rounded"
              style={{
                background: 'var(--color-input)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            />
          </div>
        </div>

        {/* Trading Leverage (Exchange) */}
        <div className="mb-2">
          <p className="text-xs font-medium mb-2" style={{ color: '#F0B90B' }}>
            {ts(riskControl.tradingLeverage, language)}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4 mb-4">
          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.btcEthLeverage, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.btcEthLeverageDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.btc_eth_max_leverage ?? 5}
                onChange={(e) =>
                  updateField('btc_eth_max_leverage', parseInt(e.target.value))
                }
                disabled={disabled}
                min={1}
                max={20}
                className="flex-1 accent-yellow-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#F0B90B' }}
              >
                {config.btc_eth_max_leverage ?? 5}x
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.altcoinLeverage, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.altcoinLeverageDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.altcoin_max_leverage ?? 5}
                onChange={(e) =>
                  updateField('altcoin_max_leverage', parseInt(e.target.value))
                }
                disabled={disabled}
                min={1}
                max={20}
                className="flex-1 accent-yellow-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#F0B90B' }}
              >
                {config.altcoin_max_leverage ?? 5}x
              </span>
            </div>
          </div>
        </div>

        {/* Position Value Ratio (Risk Control - CODE ENFORCED) */}
        <div className="mb-2">
          <p className="text-xs font-medium" style={{ color: '#0ECB81' }}>
            {ts(riskControl.positionValueRatio, language)}
          </p>
          <p className="text-xs mt-1 text-muted-foreground">
            {ts(riskControl.positionValueRatioDesc, language)}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #0ECB81',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.btcEthPositionValueRatio, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.btcEthPositionValueRatioDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.btc_eth_max_position_value_ratio ?? 5}
                onChange={(e) =>
                  updateField(
                    'btc_eth_max_position_value_ratio',
                    parseFloat(e.target.value)
                  )
                }
                disabled={disabled}
                min={0.5}
                max={10}
                step={0.5}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {config.btc_eth_max_position_value_ratio ?? 5}x
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #0ECB81',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.altcoinPositionValueRatio, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.altcoinPositionValueRatioDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.altcoin_max_position_value_ratio ?? 1}
                onChange={(e) =>
                  updateField(
                    'altcoin_max_position_value_ratio',
                    parseFloat(e.target.value)
                  )
                }
                disabled={disabled}
                min={0.5}
                max={10}
                step={0.5}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {config.altcoin_max_position_value_ratio ?? 1}x
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Risk Parameters */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <AlertTriangle className="w-5 h-5" style={{ color: '#F6465D' }} />
          <h3 className="font-medium text-foreground">
            {ts(riskControl.riskParameters, language)}
          </h3>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.minRiskReward, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.minRiskRewardDesc, language)}
            </p>
            <div className="flex items-center">
              <span className="text-muted-foreground">1:</span>
              <input
                type="number"
                name="risk-min-risk-reward-ratio"
                autoComplete="off"
                inputMode="decimal"
                value={config.min_risk_reward_ratio ?? 3}
                onChange={(e) =>
                  updateField(
                    'min_risk_reward_ratio',
                    parseFloat(e.target.value) || 3
                  )
                }
                disabled={disabled}
                min={1}
                max={10}
                step={0.5}
                className="w-20 px-3 py-2 rounded ml-2"
                style={{
                  background: 'var(--color-input)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.maxEntryPriceDeviation, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.maxEntryPriceDeviationDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.max_entry_price_deviation_pct ?? 0.5}
                onChange={(e) =>
                  updateField(
                    'max_entry_price_deviation_pct',
                    parseFloat(e.target.value)
                  )
                }
                disabled={disabled}
                min={0.1}
                max={2}
                step={0.1}
                className="flex-1 accent-yellow-500"
              />
              <span
                className="w-14 text-center font-mono"
                style={{ color: '#F0B90B' }}
              >
                {(config.max_entry_price_deviation_pct ?? 0.5).toFixed(1)}%
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #0ECB81',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.maxMarginUsage, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.maxMarginUsageDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={(config.max_margin_usage ?? 0.9) * 100}
                onChange={(e) =>
                  updateField(
                    'max_margin_usage',
                    parseInt(e.target.value) / 100
                  )
                }
                disabled={disabled}
                min={10}
                max={100}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {Math.round((config.max_margin_usage ?? 0.9) * 100)}%
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Entry Requirements */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: '#0ECB81' }} />
          <h3 className="font-medium text-foreground">
            {ts(riskControl.entryRequirements, language)}
          </h3>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.minPositionSize, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.minPositionSizeDesc, language)}
            </p>
            <div className="flex items-center">
              <input
                type="number"
                name="risk-min-position-size"
                autoComplete="off"
                inputMode="decimal"
                value={config.min_position_size ?? 12}
                onChange={(e) =>
                  updateField(
                    'min_position_size',
                    parseFloat(e.target.value) || 12
                  )
                }
                disabled={disabled}
                min={10}
                max={1000}
                className="w-24 px-3 py-2 rounded"
                style={{
                  background: 'var(--color-input)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              <span className="ml-2 text-muted-foreground">USDT</span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{
              background: 'var(--background)',
              border: '1px solid #2B3139',
            }}
          >
            <label className="block text-sm mb-1 text-foreground">
              {ts(riskControl.minConfidence, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(riskControl.minConfidenceDesc, language)}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.min_confidence ?? 75}
                onChange={(e) =>
                  updateField('min_confidence', parseInt(e.target.value))
                }
                disabled={disabled}
                min={50}
                max={100}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {config.min_confidence ?? 75}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
