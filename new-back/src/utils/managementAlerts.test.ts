import { describe, expect, it } from 'vitest'
import type { ManagementWsEvent } from '../api'
import {
  managementAlertFromEvent,
  mergeManagementAlertQueue,
  shouldPlayManagementAlertSound,
  type ManagementAlertContext,
} from './managementAlerts'

const context = (overrides: Partial<ManagementAlertContext> = {}): ManagementAlertContext => ({
  role: 'agent', path: '/', visibility: 'visible', focused: true, activeChat: null, ...overrides,
})
const chat = (overrides: Record<string, unknown> = {}): ManagementWsEvent => ({
  event_id: 'evt-chat', type: 'chat_message', data: {
    operation: 'created', sender_kind: 'member', room_type: 'service', message_id: 91,
    scope: 'user:7', room_scope: 'agent:3', game_id: 'service', ...overrides,
  },
})

describe('management realtime alerts', () => {
  it('only alerts pending application events', () => {
    const pending = managementAlertFromEvent({ type: 'application', data: { status: 'pending', request_type: 'credit', application_id: 6 } }, context())
    expect(pending).toMatchObject({ key: 'application:6', path: '/applications', audible: true })
    expect(managementAlertFromEvent({ type: 'application', data: { status: 'approved', application_id: 6 } }, context())).toBeNull()
    expect(managementAlertFromEvent({ type: 'application', data: { status: 'pending' } }, context())).toBeNull()
  })

  it('leaves join applications to tenant and agent room owners', () => {
    const join: ManagementWsEvent = { type: 'application', data: { status: 'pending', request_type: 'join', application_id: 8 } }
    expect(managementAlertFromEvent(join, context({ role: 'admin' }))).toBeNull()
    expect(managementAlertFromEvent(join, context({ role: 'tenant' }))).toMatchObject({ key: 'application:8', audible: true })
    expect(managementAlertFromEvent(join, context({ role: 'agent' }))).toMatchObject({ key: 'application:8', audible: true })
  })

  it('suppresses the exact focused service conversation', () => {
    const target = { scope: 'user:7', room_scope: 'agent:3', game_id: 'service', room_type: 'service' as const }
    expect(managementAlertFromEvent(chat(), context({ path: '/chat', activeChat: target }))).toBeNull()
    expect(managementAlertFromEvent(chat(), context({ path: '/chat', focused: false, activeChat: target }))).toMatchObject({ kind: 'service' })
    expect(managementAlertFromEvent(chat(), context({ path: '/chat', visibility: 'hidden', activeChat: target }))).toMatchObject({ kind: 'service' })
  })

  it('keeps room and lottery group traffic out of global alerts', () => {
    expect(managementAlertFromEvent(chat({ room_type: 'group', scope: 'agent:3', game_id: 'lobby' }), context())).toBeNull()
    expect(managementAlertFromEvent(chat({ room_type: 'group', scope: 'agent:3', game_id: 'speed-racing' }), context())).toBeNull()
  })

  it('ignores staff, replayed updates and malformed events', () => {
    expect(managementAlertFromEvent(chat({ sender_kind: 'staff' }), context())).toBeNull()
    expect(managementAlertFromEvent(chat({ operation: 'updated' }), context())).toBeNull()
    expect(managementAlertFromEvent(chat({ message_id: null }), context())).toBeNull()
  })

  it('deduplicates exact messages and coalesces bursts from one conversation', () => {
    const first = managementAlertFromEvent(chat(), context())!
    const duplicate = mergeManagementAlertQueue([first], first)
    expect(duplicate).toHaveLength(1)
    const next = managementAlertFromEvent(chat({ message_id: 92 }), context())!
    expect(mergeManagementAlertQueue(duplicate, next)).toEqual([{ ...next, count: 2 }])
  })

  it('plays sound only for audible live alerts after interaction and when enabled', () => {
    const alert = managementAlertFromEvent(chat(), context())!
    expect(shouldPlayManagementAlertSound(alert, true, true)).toBe(true)
    expect(shouldPlayManagementAlertSound(alert, false, true)).toBe(false)
    expect(shouldPlayManagementAlertSound(alert, true, false)).toBe(false)
    expect(shouldPlayManagementAlertSound({ ...alert, audible: false }, true, true)).toBe(false)
  })
})
