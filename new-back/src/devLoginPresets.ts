export type LoginIdentity = 'platform' | 'tenant' | 'agent'

export type DevLoginPreset = {
  workspace: string
  username: string
  password: string
}

type DevLoginEnvironment = Readonly<Record<string, string | boolean | undefined>>

// Call this only inside an import.meta.env.DEV branch so production bundles
// discard both the fixture credentials and any local environment overrides.
export function createDevLoginPresets(
  development: boolean,
  environment: DevLoginEnvironment = {},
): Partial<Record<LoginIdentity, DevLoginPreset>> {
  if (!development) return {}

  const defaults: Record<LoginIdentity, DevLoginPreset> = {
    platform: { workspace: '平台', username: 'admin', password: 'Admin8801!' },
    tenant: { workspace: '平台', username: 'wangzhetenant', password: 'WzTenant8801' },
    agent: { workspace: 'wangzhetenant', username: 'suyang', password: 'Room8801' },
  }

  const configured = (key: string, fallback: string) => {
    const value = environment[key]
    return typeof value === 'string' ? value : fallback
  }

  for (const identity of Object.keys(defaults) as LoginIdentity[]) {
    const prefix = `VITE_DEV_${identity.toUpperCase()}`
    const preset = defaults[identity]
    defaults[identity] = {
      username: configured(`${prefix}_USERNAME`, preset.username).trim(),
      workspace: configured(`${prefix}_WORKSPACE`, preset.workspace).trim(),
      password: configured(`${prefix}_PASSWORD`, preset.password),
    }
  }

  return defaults
}
