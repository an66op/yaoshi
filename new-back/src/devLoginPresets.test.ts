import { describe, expect, it } from 'vitest'
import { createDevLoginPresets } from './devLoginPresets'

describe('development management login presets', () => {
  it('matches fresh debug database identities by default', () => {
    expect(createDevLoginPresets(true)).toEqual({
      platform: { workspace: '平台', username: 'admin', password: 'Admin8801!' },
      tenant: { workspace: '平台', username: 'wangzhetenant', password: 'WzTenant8801' },
      agent: { workspace: 'wangzhetenant', username: 'suyang', password: 'Room8801' },
    })
  })

  it('allows a local standalone agent without changing the other identities', () => {
    const defaults = createDevLoginPresets(true)
    const presets = createDevLoginPresets(true, {
      VITE_DEV_AGENT_USERNAME: ' local_agent ',
      VITE_DEV_AGENT_WORKSPACE: ' 平台 ',
    })

    expect(presets.agent).toEqual({ username: 'local_agent', workspace: '平台', password: 'Room8801' })
    expect(presets.platform).toEqual(defaults.platform)
    expect(presets.tenant).toEqual(defaults.tenant)
    expect(createDevLoginPresets(true)).toEqual(defaults)
  })

  it('supports independent username, password and workspace overrides for every identity', () => {
    const environment = Object.fromEntries(['PLATFORM', 'TENANT', 'AGENT'].flatMap(identity => [
      [`VITE_DEV_${identity}_USERNAME`, `${identity.toLowerCase()}_local`],
      [`VITE_DEV_${identity}_PASSWORD`, `${identity}_local_password`],
      [`VITE_DEV_${identity}_WORKSPACE`, `${identity}_workspace`],
    ]))
    const presets = createDevLoginPresets(true, environment)

    for (const identity of ['platform', 'tenant', 'agent'] as const) {
      expect(presets[identity]).toEqual({
        username: `${identity}_local`,
        password: `${identity.toUpperCase()}_local_password`,
        workspace: `${identity.toUpperCase()}_workspace`,
      })
    }
  })

  it('preserves password whitespace and lets an explicit blank clear a preset', () => {
    const presets = createDevLoginPresets(true, {
      VITE_DEV_PLATFORM_USERNAME: '',
      VITE_DEV_PLATFORM_PASSWORD: '',
      VITE_DEV_AGENT_WORKSPACE: '',
      VITE_DEV_AGENT_PASSWORD: ' local password ',
    })

    expect(presets.platform).toEqual({ username: '', password: '', workspace: '平台' })
    expect(presets.agent).toEqual({ username: 'suyang', workspace: '', password: ' local password ' })
  })

  it('never enables presets outside development even when overrides are supplied', () => {
    expect(createDevLoginPresets(false)).toEqual({})
    expect(createDevLoginPresets(false, {
      VITE_DEV_PLATFORM_USERNAME: 'must_not_prefill',
      VITE_DEV_PLATFORM_PASSWORD: 'must_not_prefill_password',
      VITE_DEV_AGENT_WORKSPACE: 'must_not_prefill_workspace',
    })).toEqual({})
  })
})
