import { Grid, DollarSign, TrendingUp, Shield, Compass } from 'lucide-react'
import type { GridStrategyConfig } from '../../types'
import { gridConfig, ts } from '../../i18n/strategy-translations'
import { AiTSelect } from '../ui/select'

interface GridConfigEditorProps {
  config: GridStrategyConfig
  onChange: (config: GridStrategyConfig) => void
  disabled?: boolean
  language: string
}

export function GridConfigEditor({
  config,
  onChange,
  disabled,
  language,
}: GridConfigEditorProps) {
  const updateField = <K extends keyof GridStrategyConfig>(
    key: K,
    value: GridStrategyConfig[K]
  ) => {
    if (!disabled) {
      onChange({ ...config, [key]: value })
    }
  }

  const inputStyle = {
    background: 'var(--color-input)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
  }

  const sectionStyle = {
    background: 'var(--color-background)',
    border: '1px solid #2B3139',
  }

  return (
    <div className="space-y-6">
      {/* Trading Setup */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <DollarSign className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(gridConfig.tradingPair, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {/* Symbol */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.symbol, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.symbolDesc, language)}
            </p>
            <AiTSelect
              value={config.symbol}
              onChange={(val) => updateField('symbol', val)}
              disabled={disabled}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
              options={[
                { value: 'BTCUSDT', label: 'BTC/USDT' },
                { value: 'ETHUSDT', label: 'ETH/USDT' },
                { value: 'SOLUSDT', label: 'SOL/USDT' },
                { value: 'BNBUSDT', label: 'BNB/USDT' },
                { value: 'XRPUSDT', label: 'XRP/USDT' },
                { value: 'DOGEUSDT', label: 'DOGE/USDT' },
              ]}
            />
          </div>

          {/* Investment */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.totalInvestment, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.totalInvestmentDesc, language)}
            </p>
            <input
              type="number"
              name="grid-total-investment"
              autoComplete="off"
              inputMode="decimal"
              value={config.total_investment}
              onChange={(e) =>
                updateField(
                  'total_investment',
                  parseFloat(e.target.value) || 1000
                )
              }
              disabled={disabled}
              min={100}
              step={100}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          {/* Leverage */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.leverage, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.leverageDesc, language)}
            </p>
            <input
              type="number"
              name="grid-leverage"
              autoComplete="off"
              inputMode="numeric"
              value={config.leverage}
              onChange={(e) =>
                updateField('leverage', parseInt(e.target.value) || 5)
              }
              disabled={disabled}
              min={1}
              max={5}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        </div>
      </div>

      {/* Grid Parameters */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Grid className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(gridConfig.gridParameters, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Grid Count */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.gridCount, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.gridCountDesc, language)}
            </p>
            <input
              type="number"
              name="grid-count"
              autoComplete="off"
              inputMode="numeric"
              value={config.grid_count}
              onChange={(e) =>
                updateField('grid_count', parseInt(e.target.value) || 10)
              }
              disabled={disabled}
              min={5}
              max={50}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          {/* Distribution */}
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.distribution, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.distributionDesc, language)}
            </p>
            <AiTSelect
              value={config.distribution}
              onChange={(val) =>
                updateField(
                  'distribution',
                  val as 'uniform' | 'gaussian' | 'pyramid'
                )
              }
              disabled={disabled}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
              options={[
                { value: 'uniform', label: ts(gridConfig.uniform, language) },
                { value: 'gaussian', label: ts(gridConfig.gaussian, language) },
                { value: 'pyramid', label: ts(gridConfig.pyramid, language) },
              ]}
            />
          </div>
        </div>
      </div>

      {/* Price Bounds */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <TrendingUp className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(gridConfig.priceBounds, language)}
          </h3>
        </div>

        {/* ATR Toggle */}
        <div className="p-4 rounded-lg mb-4" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm text-foreground">
                {ts(gridConfig.useAtrBounds, language)}
              </label>
              <p className="text-xs text-muted-foreground">
                {ts(gridConfig.useAtrBoundsDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_atr_bounds}
                onChange={(e) =>
                  updateField('use_atr_bounds', e.target.checked)
                }
                disabled={disabled}
                className="sr-only peer"
              />
              <div className="w-11 h-6 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
            </label>
          </div>
        </div>

        {config.use_atr_bounds ? (
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.atrMultiplier, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.atrMultiplierDesc, language)}
            </p>
            <input
              type="number"
              name="grid-atr-multiplier"
              autoComplete="off"
              inputMode="decimal"
              value={config.atr_multiplier}
              onChange={(e) =>
                updateField('atr_multiplier', parseFloat(e.target.value) || 2.0)
              }
              disabled={disabled}
              min={1}
              max={5}
              step={0.5}
              className="w-32 px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1 text-foreground">
                {ts(gridConfig.upperPrice, language)}
              </label>
              <p className="text-xs mb-2 text-muted-foreground">
                {ts(gridConfig.upperPriceDesc, language)}
              </p>
              <input
                type="number"
                name="grid-upper-price"
                autoComplete="off"
                inputMode="decimal"
                value={config.upper_price}
                onChange={(e) =>
                  updateField('upper_price', parseFloat(e.target.value) || 0)
                }
                disabled={disabled}
                min={0}
                step={0.01}
                className="w-full px-3 py-2 rounded"
                style={inputStyle}
              />
            </div>
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1 text-foreground">
                {ts(gridConfig.lowerPrice, language)}
              </label>
              <p className="text-xs mb-2 text-muted-foreground">
                {ts(gridConfig.lowerPriceDesc, language)}
              </p>
              <input
                type="number"
                name="grid-lower-price"
                autoComplete="off"
                inputMode="decimal"
                value={config.lower_price}
                onChange={(e) =>
                  updateField('lower_price', parseFloat(e.target.value) || 0)
                }
                disabled={disabled}
                min={0}
                step={0.01}
                className="w-full px-3 py-2 rounded"
                style={inputStyle}
              />
            </div>
          </div>
        )}
      </div>

      {/* Risk Control */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(gridConfig.riskControl, language)}
          </h3>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.maxDrawdown, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.maxDrawdownDesc, language)}
            </p>
            <input
              type="number"
              name="grid-max-drawdown-pct"
              autoComplete="off"
              inputMode="decimal"
              value={config.max_drawdown_pct}
              onChange={(e) =>
                updateField(
                  'max_drawdown_pct',
                  parseFloat(e.target.value) || 15
                )
              }
              disabled={disabled}
              min={5}
              max={50}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.stopLoss, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.stopLossDesc, language)}
            </p>
            <input
              type="number"
              name="grid-stop-loss-pct"
              autoComplete="off"
              inputMode="decimal"
              value={config.stop_loss_pct}
              onChange={(e) =>
                updateField('stop_loss_pct', parseFloat(e.target.value) || 5)
              }
              disabled={disabled}
              min={1}
              max={20}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>

          <div className="p-4 rounded-lg" style={sectionStyle}>
            <label className="block text-sm mb-1 text-foreground">
              {ts(gridConfig.dailyLossLimit, language)}
            </label>
            <p className="text-xs mb-2 text-muted-foreground">
              {ts(gridConfig.dailyLossLimitDesc, language)}
            </p>
            <input
              type="number"
              name="grid-daily-loss-limit-pct"
              autoComplete="off"
              inputMode="decimal"
              value={config.daily_loss_limit_pct}
              onChange={(e) =>
                updateField(
                  'daily_loss_limit_pct',
                  parseFloat(e.target.value) || 10
                )
              }
              disabled={disabled}
              min={1}
              max={30}
              className="w-full px-3 py-2 rounded"
              style={inputStyle}
            />
          </div>
        </div>

        {/* Maker Only Toggle */}
        <div className="p-4 rounded-lg" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm text-foreground">
                {ts(gridConfig.useMakerOnly, language)}
              </label>
              <p className="text-xs text-muted-foreground">
                {ts(gridConfig.useMakerOnlyDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.use_maker_only}
                onChange={(e) =>
                  updateField('use_maker_only', e.target.checked)
                }
                disabled={disabled}
                className="sr-only peer"
              />
              <div className="w-11 h-6 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
            </label>
          </div>
        </div>
      </div>

      {/* Direction Auto-Adjust */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Compass className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium text-foreground">
            {ts(gridConfig.directionAdjust, language)}
          </h3>
        </div>

        {/* Enable Toggle */}
        <div className="p-4 rounded-lg mb-4" style={sectionStyle}>
          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm text-foreground">
                {ts(gridConfig.enableDirectionAdjust, language)}
              </label>
              <p className="text-xs text-muted-foreground">
                {ts(gridConfig.enableDirectionAdjustDesc, language)}
              </p>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={config.enable_direction_adjust ?? false}
                onChange={(e) =>
                  updateField('enable_direction_adjust', e.target.checked)
                }
                disabled={disabled}
                className="sr-only peer"
              />
              <div className="w-11 h-6 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
            </label>
          </div>
        </div>

        {config.enable_direction_adjust && (
          <>
            {/* Direction Modes Explanation */}
            <div
              className="p-4 rounded-lg mb-4"
              style={{ background: '#1E2329', border: '1px solid #F0B90B33' }}
            >
              <p
                className="text-xs font-medium mb-2"
                style={{ color: '#F0B90B' }}
              >
                📊 {ts(gridConfig.directionModes, language)}
              </p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs text-muted-foreground">
                <div>• {ts(gridConfig.modeNeutral, language)}</div>
                <div>
                  •{' '}
                  <span style={{ color: '#0ECB81' }}>
                    {ts(gridConfig.modeLongBias, language)}
                  </span>
                </div>
                <div>
                  •{' '}
                  <span style={{ color: '#0ECB81' }}>
                    {ts(gridConfig.modeLong, language)}
                  </span>
                </div>
                <div>
                  •{' '}
                  <span style={{ color: '#F6465D' }}>
                    {ts(gridConfig.modeShortBias, language)}
                  </span>
                </div>
                <div>
                  •{' '}
                  <span style={{ color: '#F6465D' }}>
                    {ts(gridConfig.modeShort, language)}
                  </span>
                </div>
              </div>
              <p className="text-xs mt-3 pt-2 border-t border-border text-muted-foreground">
                💡 {ts(gridConfig.directionExplain, language)}
              </p>
            </div>

            {/* Bias Strength */}
            <div className="p-4 rounded-lg" style={sectionStyle}>
              <label className="block text-sm mb-1 text-foreground">
                {ts(gridConfig.directionBiasRatio, language)} (X)
              </label>
              <p className="text-xs mb-1 text-muted-foreground">
                {ts(gridConfig.directionBiasRatioDesc, language)}
              </p>
              <p className="text-xs mb-3" style={{ color: '#F0B90B' }}>
                {ts(gridConfig.directionBiasExplain, language)}
              </p>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  value={(config.direction_bias_ratio ?? 0.7) * 100}
                  onChange={(e) =>
                    updateField(
                      'direction_bias_ratio',
                      parseInt(e.target.value) / 100
                    )
                  }
                  disabled={disabled}
                  min={55}
                  max={90}
                  step={5}
                  className="flex-1 h-2 rounded-lg appearance-none cursor-pointer"
                  style={{ background: '#2B3139' }}
                />
                <span
                  className="text-sm font-mono w-20 text-right"
                  style={{ color: '#F0B90B' }}
                >
                  X = {Math.round((config.direction_bias_ratio ?? 0.7) * 100)}%
                </span>
              </div>
              <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
                <div
                  className="p-2 rounded"
                  style={{
                    background: '#0ECB8115',
                    border: '1px solid #0ECB8130',
                  }}
                >
                  <span style={{ color: '#0ECB81' }}>Long Bias: </span>
                  <span className="text-foreground">
                    {Math.round((config.direction_bias_ratio ?? 0.7) * 100)}%{' '}
                    {ts(gridConfig.buy, language)} +{' '}
                    {Math.round(
                      (1 - (config.direction_bias_ratio ?? 0.7)) * 100
                    )}
                    % {ts(gridConfig.sell, language)}
                  </span>
                </div>
                <div
                  className="p-2 rounded"
                  style={{
                    background: '#F6465D15',
                    border: '1px solid #F6465D30',
                  }}
                >
                  <span style={{ color: '#F6465D' }}>Short Bias: </span>
                  <span className="text-foreground">
                    {Math.round(
                      (1 - (config.direction_bias_ratio ?? 0.7)) * 100
                    )}
                    % {ts(gridConfig.buy, language)} +{' '}
                    {Math.round((config.direction_bias_ratio ?? 0.7) * 100)}%{' '}
                    {ts(gridConfig.sell, language)}
                  </span>
                </div>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
