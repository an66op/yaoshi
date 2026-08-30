import { describe, expect, it } from 'vitest'
import { formatBetAmount } from './betAmount'

describe('stake amount display', () => {
  it.each([[0, '0'], [20, '20'], [50, '50'], [550, '550'], [10000, '10000'], [0.5, '0.50'], [1.25, '1.25'], [12.01, '12.01']] as const)(
    'formats %s as %s without losing real cents', (amount, expected) => expect(formatBetAmount(amount)).toBe(expected),
  )
})
