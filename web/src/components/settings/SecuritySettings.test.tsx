// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SecuritySettings } from './SecuritySettings'
import { mfaApi } from '../../lib/mfa'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('../../lib/mfa', () => ({
  mfaApi: {
    status: vi.fn(),
    setup: vi.fn(),
    enable: vi.fn(),
    disable: vi.fn(),
    verifyLogin: vi.fn(),
  },
  normalizeCode: (value: string) => value.replace(/\s+/g, '').toUpperCase(),
}))

vi.mock('../../contexts/LanguageContext', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

const refreshSessionToken = vi.fn()
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'trader@example.com' },
    refreshSessionToken,
  }),
}))

const api = vi.mocked(mfaApi)

const SETUP = {
  secret: 'JBSWY3DPEHPK3PXP',
  otpauth_uri:
    'otpauth://totp/AiT:trader@example.com?secret=JBSWY3DPEHPK3PXP&issuer=AiT&algorithm=SHA1&digits=6&period=30',
  issuer: 'AiT',
}

const RECOVERY = Array.from({ length: 10 }, (_, i) => `CODE${i}AAAAAAA`)

const digitBoxes = () =>
  [1, 2, 3, 4, 5, 6].map((n) => screen.getByLabelText(`Digit ${n}`))

function typeCode(code: string) {
  const boxes = digitBoxes()
  code.split('').forEach((digit, i) => {
    fireEvent.change(boxes[i], { target: { value: digit } })
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SecuritySettings', () => {
  it('shows the disabled state and an enable button', async () => {
    api.status.mockResolvedValue({ ok: true, data: { enabled: false } })
    render(<SecuritySettings />)

    expect(await screen.findByText('Not enabled')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Set up' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Turn off' })).toBeNull()
  })

  it('shows the enabled state and a turn-off button', async () => {
    api.status.mockResolvedValue({ ok: true, data: { enabled: true } })
    render(<SecuritySettings />)

    expect(await screen.findByText('Enabled')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Turn off' })).toBeInTheDocument()
  })

  it('walks setup -> QR -> confirm -> recovery codes', async () => {
    api.status.mockResolvedValue({ ok: true, data: { enabled: false } })
    api.setup.mockResolvedValue({ ok: true, data: SETUP })
    api.enable.mockResolvedValue({
      ok: true,
      data: {
        message: 'MFA enabled',
        recovery_codes: RECOVERY,
        token: 'rotated-jwt',
      },
    })

    const { container } = render(<SecuritySettings />)
    fireEvent.click(await screen.findByRole('button', { name: 'Set up' }))

    // QR + manual key
    await waitFor(() => expect(api.setup).toHaveBeenCalledTimes(1))
    expect(await screen.findByText(SETUP.secret)).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeTruthy()

    // Confirm with a 6-digit code
    typeCode('123456')
    await waitFor(() =>
      expect(api.enable).toHaveBeenCalledWith(SETUP.secret, '123456')
    )

    // One-time recovery-code reveal
    expect(await screen.findByText('Save your recovery codes')).toBeInTheDocument()
    RECOVERY.forEach((code) => {
      expect(screen.getByText(code)).toBeInTheDocument()
    })
    expect(refreshSessionToken).toHaveBeenCalledWith('rotated-jwt')

    // Dismissing returns to the enabled summary
    fireEvent.click(screen.getByRole('button', { name: 'I have saved them' }))
    expect(await screen.findByText('Enabled')).toBeInTheDocument()
  })

  it('surfaces an error and stays in setup when the code is wrong', async () => {
    api.status.mockResolvedValue({ ok: true, data: { enabled: false } })
    api.setup.mockResolvedValue({ ok: true, data: SETUP })
    api.enable.mockResolvedValue({
      ok: false,
      error: 'Invalid authenticator code',
    })

    render(<SecuritySettings />)
    fireEvent.click(await screen.findByRole('button', { name: 'Set up' }))
    await screen.findByText(SETUP.secret)

    typeCode('000000')
    expect(
      await screen.findByText('Invalid authenticator code')
    ).toBeInTheDocument()
    // Still on the setup step, not enabled.
    expect(screen.getByText(SETUP.secret)).toBeInTheDocument()
  })

  it('disables MFA after a correct code', async () => {
    api.status.mockResolvedValue({ ok: true, data: { enabled: true } })
    api.disable.mockResolvedValue({ ok: true, data: { message: 'MFA disabled' } })

    render(<SecuritySettings />)
    fireEvent.click(await screen.findByRole('button', { name: 'Turn off' }))

    expect(await screen.findByText('Turn off two-factor authentication')).toBeInTheDocument()
    typeCode('654321')
    fireEvent.click(screen.getByRole('button', { name: 'Turn off' }))

    await waitFor(() => expect(api.disable).toHaveBeenCalledWith('654321'))
    expect(await screen.findByText('Not enabled')).toBeInTheDocument()
  })

  it('reports a failed status load with a retry', async () => {
    api.status.mockResolvedValue({ ok: false, error: 'Network error' })
    render(<SecuritySettings />)

    expect(await screen.findByText('Network error')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})
