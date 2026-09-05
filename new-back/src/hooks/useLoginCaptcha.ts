import { useCallback, useEffect, useRef, useState } from 'react'
import { adminApi } from '../api'

type Challenge = { id: string; image: string; expiresAt: number; requestID: number }
type CaptchaState = {
  status: 'loading' | 'image' | 'ready' | 'error' | 'used'
  challenge: Challenge | null
  message: string
}

export const LOGIN_CAPTCHA_LENGTH = 4

/** A challenge belongs to one mounted login form, never to a stored test profile. */
export function useLoginCaptcha() {
  const [captcha, setCaptcha] = useState<CaptchaState>({ status: 'loading', challenge: null, message: '' })
  const [code, setCode] = useState('')
  const current = useRef(captcha)
  const mounted = useRef(false)
  const request = useRef<AbortController | null>(null)
  const requestTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const sequence = useRef(0)
  const expiryTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const refreshCurrent = useRef<() => void>(() => undefined)

  const update = useCallback((next: CaptchaState) => { current.current = next; setCaptcha(next) }, [])
  const clearExpiry = useCallback(() => { clearTimeout(expiryTimer.current); expiryTimer.current = undefined }, [])
  const expire = useCallback(() => {
    clearExpiry()
    setCode('')
    refreshCurrent.current()
  }, [clearExpiry])

  const refresh = useCallback(async () => {
    if (!mounted.current) return
    const requestID = ++sequence.current
    request.current?.abort()
    clearTimeout(requestTimer.current)
    clearExpiry()
    const controller = new AbortController()
    request.current = controller
    setCode('')
    update({ status: 'loading', challenge: null, message: '' })
    // Start the client validity window before the request, so a slow response
    // cannot make a server-side expired challenge appear usable for longer.
    const startedAt = Date.now()
    const timeout = setTimeout(() => {
      if (!mounted.current || sequence.current !== requestID) return
      controller.abort()
      update({ status: 'error', challenge: null, message: '验证码加载超时，请点击换一张重试' })
    }, 15_000)
    requestTimer.current = timeout
    try {
      const result = await adminApi.loginCaptcha(controller.signal)
      if (!mounted.current || controller.signal.aborted || sequence.current !== requestID) return
      if (!result || typeof result.id !== 'string' || !result.id.trim() ||
        typeof result.image !== 'string' || !/^data:image\/png;base64,[a-zA-Z0-9+/]+=*$/.test(result.image) ||
        !Number.isFinite(result.expires_in) || result.expires_in <= 0) throw new Error('Invalid captcha response')
      const challenge = { id: result.id, image: result.image, expiresAt: startedAt + Math.min(result.expires_in, 120) * 1000, requestID }
      update({ status: 'image', challenge, message: '' })
      if (Date.now() >= challenge.expiresAt) { expire(); return }
      expiryTimer.current = setTimeout(() => {
        if (mounted.current && sequence.current === requestID && current.current.status !== 'used') expire()
      }, challenge.expiresAt - Date.now())
    } catch {
      if (!mounted.current || controller.signal.aborted || sequence.current !== requestID) return
      update({ status: 'error', challenge: null, message: '验证码加载失败，请点击换一张重试' })
    } finally {
      clearTimeout(timeout)
      if (requestTimer.current === timeout) requestTimer.current = undefined
      if (request.current === controller) request.current = null
    }
  }, [clearExpiry, expire, update])
  useEffect(() => {
    refreshCurrent.current = () => { void refresh() }
    return () => { refreshCurrent.current = () => undefined }
  }, [refresh])

  useEffect(() => {
    mounted.current = true
    void refresh()
    return () => {
      mounted.current = false
      sequence.current += 1
      request.current?.abort()
      clearTimeout(requestTimer.current)
      clearExpiry()
    }
  }, [clearExpiry, refresh])

  const imageLoaded = (requestID: number) => {
    if (!mounted.current || current.current.status !== 'image' || current.current.challenge?.requestID !== requestID) return
    if (Date.now() >= current.current.challenge.expiresAt) { expire(); return }
    update({ ...current.current, status: 'ready' })
  }
  const imageFailed = (requestID: number) => {
    if (!mounted.current || current.current.challenge?.requestID !== requestID || current.current.status === 'used') return
    clearExpiry()
    setCode('')
    update({ status: 'error', challenge: null, message: '验证码图片无法显示，请点击换一张重试' })
  }
  const takeSubmission = () => {
    const challenge = current.current.challenge
    if (challenge && Date.now() >= challenge.expiresAt) { expire(); throw new Error('验证码已过期，已自动刷新') }
    if (current.current.status !== 'ready' || !challenge) throw new Error('请先加载验证码图片')
    if (!new RegExp(`^\\d{${LOGIN_CAPTCHA_LENGTH}}$`).test(code)) throw new Error(`请输入图中${LOGIN_CAPTCHA_LENGTH}位数字验证码`)
    clearExpiry()
    update({ ...current.current, status: 'used' })
    setCode('')
    return { captcha_id: challenge.id, captcha_code: code }
  }

  return { captcha, code, setCode, refresh, imageLoaded, imageFailed, takeSubmission, mounted }
}
