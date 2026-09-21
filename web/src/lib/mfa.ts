/**
 * TOTP MFA client.
 *
 * Deliberately bypasses `httpClient`. Its response interceptor treats *every*
 * 401 as a dead session — it wipes localStorage, dispatches `unauthorized` and
 * bounces the browser to /login. That is exactly wrong here: a mistyped
 * authenticator code legitimately answers 401 and must be rendered inline
 * instead of logging the user out mid-flow. These calls therefore use raw
 * fetch and surface `{ error }` to the caller.
 *
 * Backend contract (api/handler_mfa.go):
 *   GET  /api/mfa/status         (auth)  -> { enabled }
 *   POST /api/mfa/setup          (auth)  -> { secret, otpauth_uri, issuer }
 *   POST /api/mfa/enable         (auth)  -> { message, recovery_codes[10], token }
 *   POST /api/mfa/disable        (auth)  -> { message }
 *   POST /api/mfa/login/verify   (public, rate-limited) -> { token, user_id, email }
 *
 * NOTE: the login challenge survives a wrong code. The server keeps it until
 * it is redeemed, expires, or `maxMFAAttempts` wrong codes have been tried, so
 * a typo is retryable in place. Failures carry a machine-readable `reason`:
 *   expired | unavailable | too_many_attempts -> terminal, restart the login
 *   invalid_code                              -> retryable, stay on the step
 * `attemptsLeft` is present while the challenge is still alive.
 */

export interface MfaStatus {
  enabled: boolean
}

export interface MfaSetupPayload {
  secret: string
  otpauth_uri: string
  issuer: string
}

export interface MfaEnablePayload {
  message: string
  recovery_codes: string[]
  token: string
}

export interface MfaLoginPayload {
  token: string
  user_id: string
  email: string
  message?: string
}

/** Machine-readable failure causes returned by `POST /api/mfa/login/verify`. */
export type MfaReason =
  | 'expired'
  | 'unavailable'
  | 'invalid_code'
  | 'too_many_attempts'

export type MfaResult<T> =
  | { ok: true; data: T }
  | {
      ok: false
      error: string
      /** Present on MFA verify failures; absent for transport/other errors. */
      reason?: MfaReason
      /** Guesses left on the current challenge, when the server reports it. */
      attemptsLeft?: number
      /**
       * HTTP status. Needed because a 429 is ambiguous: the MFA handler sends
       * one with `reason: 'too_many_attempts'`, while the shared IP limiter
       * (`authRateLimiter`, 10 req / 15 min) sends one with no reason at all.
       */
      status?: number
    }

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('auth_token')
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  return headers
}

async function call<T>(
  path: string,
  method: string,
  options: { body?: unknown; auth?: boolean } = {}
): Promise<MfaResult<T>> {
  const { body, auth = true } = options
  try {
    const response = await fetch(path, {
      method,
      headers: auth ? authHeaders() : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })

    const raw = await response.text()
    let payload: Record<string, unknown> | null = null
    if (raw) {
      try {
        payload = JSON.parse(raw)
      } catch {
        payload = null
      }
    }

    if (!response.ok) {
      const message =
        (payload?.error as string) ||
        (payload?.message as string) ||
        `Request failed (${response.status})`
      return {
        ok: false,
        error: message,
        reason: (payload?.reason as MfaReason | undefined) ?? undefined,
        attemptsLeft:
          typeof payload?.attempts_left === 'number'
            ? (payload.attempts_left as number)
            : undefined,
        status: response.status,
      }
    }

    return { ok: true, data: (payload ?? {}) as T }
  } catch {
    return { ok: false, error: 'Network error, please try again' }
  }
}

export const mfaApi = {
  status: () => call<MfaStatus>('/api/mfa/status', 'GET'),

  setup: () => call<MfaSetupPayload>('/api/mfa/setup', 'POST'),

  enable: (secret: string, code: string) =>
    call<MfaEnablePayload>('/api/mfa/enable', 'POST', {
      body: { secret, code },
    }),

  disable: (code: string) =>
    call<{ message: string }>('/api/mfa/disable', 'POST', {
      body: { code },
    }),

  /** Public endpoint — no bearer token is expected on the login screen. */
  verifyLogin: (mfaToken: string, code: string) =>
    call<MfaLoginPayload>('/api/mfa/login/verify', 'POST', {
      body: { mfa_token: mfaToken, code },
      auth: false,
    }),
}

/** Normalises whatever the user typed into the shape the server compares against. */
export function normalizeCode(input: string): string {
  return input.replace(/\s+/g, '').toUpperCase()
}

/** TOTP codes are 6 digits; recovery codes are 13-char base32 (8 random bytes). */
export function looksLikeRecoveryCode(input: string): boolean {
  const value = normalizeCode(input)
  return value.length > 6
}
