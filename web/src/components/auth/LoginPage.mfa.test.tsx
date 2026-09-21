// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { LoginPage } from './LoginPage'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), dismiss: vi.fn() },
}))

const navigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => navigate,
}))

vi.mock('../../contexts/LanguageContext', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

vi.mock('../common/DeepVoidBackground', () => ({
  DeepVoidBackground: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('../common/LanguageSwitcher', () => ({
  LanguageSwitcher: () => null,
}))

const login = vi.fn()
const verifyMfa = vi.fn()
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ login, verifyMfa }),
}))

const digitBoxes = () =>
  [1, 2, 3, 4, 5, 6].map((n) => screen.getByLabelText(`Digit ${n}`))

async function submitCredentials() {
  fireEvent.change(screen.getByPlaceholderText('you@example.com'), {
    target: { value: 'trader@example.com' },
  })
  fireEvent.change(screen.getByPlaceholderText('••••••••'), {
    target: { value: 'hunter2hunter2' },
  })
  fireEvent.click(screen.getByRole('button', { name: /Sign In/ }))
}

function typeCode(code: string) {
  const boxes = digitBoxes()
  code.split('').forEach((digit, i) => {
    fireEvent.change(boxes[i], { target: { value: digit } })
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('LoginPage — MFA second step', () => {
  it('starts on the password step with no code boxes', () => {
    render(<LoginPage />)
    expect(screen.getByText('Welcome back')).toBeInTheDocument()
    expect(screen.queryByLabelText('Digit 1')).toBeNull()
  })

  it('switches to the code step when the server answers mfa_required', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
      message: 'Authenticator code required',
    })

    render(<LoginPage />)
    await submitCredentials()

    expect(
      await screen.findByText('Two-factor authentication')
    ).toBeInTheDocument()
    expect(screen.getAllByLabelText(/^Digit \d$/)).toHaveLength(6)
    // Password step is gone.
    expect(screen.queryByPlaceholderText('you@example.com')).toBeNull()
    // A 5-minute countdown is displayed.
    expect(screen.getByText(/Code expires in/)).toBeInTheDocument()
  })

  it('completes login when a correct code is entered', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa.mockResolvedValue({ success: true })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('123456')

    await waitFor(() =>
      expect(verifyMfa).toHaveBeenCalledWith('challenge-1', '123456')
    )
    // Success is handed to AuthContext, which performs the navigation.
    expect(screen.getByLabelText('Digit 1')).toBeInTheDocument()
  })

  it('keeps the code step after a wrong code so a typo can be retried', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa.mockResolvedValue({
      success: false,
      message: 'Invalid authenticator or recovery code',
      reason: 'invalid_code',
      attemptsLeft: 4,
    })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('000000')

    // The challenge survives a wrong code server-side, so the user stays put.
    expect(
      await screen.findByText('Invalid authenticator or recovery code')
    ).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('you@example.com')).toBeNull()
    expect(screen.getAllByLabelText(/^Digit \d$/)).toHaveLength(6)
    // Boxes are cleared so the next code can be typed straight away.
    expect((screen.getByLabelText('Digit 1') as HTMLInputElement).value).toBe('')
    // And the remaining budget is surfaced.
    expect(screen.getByText('Attempts remaining: 4')).toBeInTheDocument()
  })

  it('can retry after a wrong code without re-entering the password', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa
      .mockResolvedValueOnce({
        success: false,
        message: 'Invalid authenticator or recovery code',
        reason: 'invalid_code',
        attemptsLeft: 4,
      })
      .mockResolvedValueOnce({ success: true })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('000000')
    await screen.findByText('Invalid authenticator or recovery code')

    typeCode('123456')
    await waitFor(() =>
      expect(verifyMfa).toHaveBeenLastCalledWith('challenge-1', '123456')
    )
  })

  it('returns to the password step once the challenge is exhausted', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa.mockResolvedValue({
      success: false,
      message: 'Too many invalid codes; sign in again',
      reason: 'too_many_attempts',
      attemptsLeft: 0,
    })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('000000')

    await waitFor(() =>
      expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Digit 1')).toBeNull()
  })

  it('returns to the password step when the shared IP limiter rejects us', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    // The IP limiter answers 429 with no `reason`; unlike the MFA handler's own
    // 429 this means every auth endpoint is closed, so retrying here is futile.
    verifyMfa.mockResolvedValue({
      success: false,
      message: 'Too many attempts, please try again later',
      status: 429,
    })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('000000')

    await waitFor(() =>
      expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Digit 1')).toBeNull()
    expect(screen.getByText(/Too many attempts from this network/)).toBeInTheDocument()
  })

  it('returns to the password step when the challenge has expired', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa.mockResolvedValue({
      success: false,
      message: 'MFA verification expired; sign in again',
      reason: 'expired',
    })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    typeCode('000000')

    await waitFor(() =>
      expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Digit 1')).toBeNull()
  })

  it('accepts a longer recovery code when the toggle is used', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })
    verifyMfa.mockResolvedValue({ success: false, message: 'nope' })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    fireEvent.click(screen.getByRole('button', { name: /recovery code/i }))

    const recovery = screen.getByPlaceholderText('Enter one of your recovery codes')
    fireEvent.change(recovery, { target: { value: 'code1aaaaaaa' } })
    fireEvent.click(screen.getByRole('button', { name: 'Verify' }))

    // LoginPage forwards the value as typed; normalisation (upper-case, strip
    // whitespace) happens inside AuthContext.verifyMfa, which is mocked here.
    await waitFor(() =>
      expect(verifyMfa).toHaveBeenCalledWith('challenge-1', 'code1aaaaaaa')
    )
  })

  it('lets the user abandon the code step', async () => {
    login.mockResolvedValue({
      success: false,
      mfaRequired: true,
      mfaToken: 'challenge-1',
    })

    render(<LoginPage />)
    await submitCredentials()
    await screen.findByLabelText('Digit 1')

    fireEvent.click(screen.getByRole('button', { name: /Back to sign in/ }))
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
  })

  it('shows a plain login error when the password is wrong', async () => {
    login.mockResolvedValue({ success: false, message: 'Invalid credentials' })

    render(<LoginPage />)
    await submitCredentials()

    expect(await screen.findByText('Invalid credentials')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument()
  })
})
