import type { DrawResult } from '../api/lottery'
import type { Game } from '../types'
import { lotteryResultSummary, lotteryRuleProfile } from './lotteryRules'

// Result-card artwork is shared by every lottery; game identity only supplies
// the title. It must not be used as a game Logo or gated on a specific game ID.
const racingColors = ['#a8afb4', '#f3c51f', '#1d83d2', '#33383e', '#f07b20', '#35c2c4', '#6144cc', '#b8bec2', '#e33b31', '#921f1d', '#23a74e']

export const CURRENT_DRAW_CARD_SIZE = { width: 720, height: 450 } as const
const RECENT_DRAW_ROW_LIMIT = 15
const RECENT_DRAW_HEADER_HEIGHT = 84
const RECENT_DRAW_ROW_HEIGHT = 35
const RECENT_DRAW_TABLE_TOP = RECENT_DRAW_HEADER_HEIGHT + 22

function currentIssueLines(issue: string) {
  return (`第 ${drawCardIssueLabel(issue)} 期 · 开奖结果`).match(/.{1,55}/gu) ?? ['—']
}

export function currentDrawCardSize(numberCount: number, issue = '') {
  return { width: CURRENT_DRAW_CARD_SIZE.width, height: CURRENT_DRAW_CARD_SIZE.height + Math.max(0, Math.ceil(numberCount / 10) - 1) * 58 + (currentIssueLines(issue).length - 1) * 20 }
}

function issueLines(issue: string) {
  return drawCardIssueLabel(issue).match(/.{1,19}/gu) ?? ['—']
}

function recentRowHeight(draw: DrawResult) {
  return Math.max(RECENT_DRAW_ROW_HEIGHT, Math.ceil(draw.numbers.length / 10) * 30 + 5, issueLines(draw.issue).length * 16 + 15)
}

export function recentDrawCardSize(draws: number | DrawResult[]) {
  const contentHeight = typeof draws === 'number'
    ? Math.min(RECENT_DRAW_ROW_LIMIT, Math.max(0, draws)) * RECENT_DRAW_ROW_HEIGHT
    : draws.slice(0, RECENT_DRAW_ROW_LIMIT).reduce((height, draw) => height + recentRowHeight(draw), 0)
  return { width: 720, height: RECENT_DRAW_TABLE_TOP + contentHeight + 14 }
}

export function releaseDrawCardCanvas(canvas: HTMLCanvasElement) {
  // Clearing pixels retains the allocation; resetting dimensions releases it.
  // Display size is independently reserved through the canvas aspect-ratio.
  if (canvas.width !== 0) canvas.width = 0
  if (canvas.height !== 0) canvas.height = 0
}

function drawBall(ctx: CanvasRenderingContext2D, value: number, x: number, y: number, size: number) {
  ctx.fillStyle = racingColors[value] ?? '#1596a7'
  ctx.beginPath()
  ctx.roundRect(x, y, size, size, Math.max(4, size * .18))
  ctx.fill()
  ctx.fillStyle = value === 1 || value === 7 ? '#172c35' : '#fff'
  ctx.font = `800 ${Math.round(size * .53)}px Arial, sans-serif`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(String(value), x + size / 2, y + size / 2 + 1)
}

export function drawCardIssueLabel(issue: string) {
  return issue
}

function prepareCanvas(canvas: HTMLCanvasElement, width: number, height: number, pixelRatio: number) {
  const ratio = Math.max(1, Math.min(pixelRatio || 1, 2))
  canvas.width = width * ratio
  canvas.height = height * ratio
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  ctx.scale(ratio, ratio)
  return ctx
}

function drawPodiumMarker(ctx: CanvasRenderingContext2D, value: number, x: number, y: number, rank: string, champion: boolean) {
  ctx.save()
  ctx.shadowColor = champion ? 'rgba(255,214,80,.6)' : 'rgba(50,207,221,.38)'
  ctx.shadowBlur = champion ? 18 : 11
  ctx.fillStyle = champion ? 'rgba(255,207,60,.94)' : 'rgba(7,48,69,.9)'
  ctx.strokeStyle = champion ? '#fff1a8' : '#8fe7ea'
  ctx.lineWidth = 2
  ctx.beginPath(); ctx.arc(x, y, champion ? 22 : 19, 0, Math.PI * 2); ctx.fill(); ctx.stroke()
  ctx.shadowBlur = 0
  ctx.fillStyle = champion ? '#173345' : '#fff'
  ctx.textAlign = 'center'; ctx.textBaseline = 'middle'
  ctx.font = `900 ${champion ? 21 : 18}px Arial, sans-serif`
  ctx.fillText(String(value), x, y + 1)
  ctx.fillStyle = champion ? '#ffe47b' : '#c8f2f3'
  ctx.font = '800 13px Arial, sans-serif'
  ctx.fillText(rank, x, y - 31)
  ctx.restore()
}

export function paintCurrentDrawCard(canvas: HTMLCanvasElement, game: Pick<Game, 'title'> & Partial<Pick<Game, 'ruleVersion'>>, draw: DrawResult, racingCars: CanvasImageSource | null, pixelRatio = 1) {
  const { width, height } = currentDrawCardSize(draw.numbers.length, draw.issue)
  const extraHeight = height - CURRENT_DRAW_CARD_SIZE.height
  const headerExtraHeight = (currentIssueLines(draw.issue).length - 1) * 20
  const ctx = prepareCanvas(canvas, width, height, pixelRatio)
  if (!ctx) return
  const background = ctx.createLinearGradient(0, 0, width, height)
  background.addColorStop(0, '#102f48')
  background.addColorStop(.55, '#176b7f')
  background.addColorStop(1, '#3baca6')
  ctx.fillStyle = background
  ctx.fillRect(0, 0, width, height)
  ctx.fillStyle = 'rgba(255,255,255,.09)'
  for (let x = -100; x < width; x += 90) {
    ctx.beginPath()
    ctx.moveTo(x, 0)
    ctx.lineTo(x + 190, height)
    ctx.lineWidth = 28
    ctx.strokeStyle = 'rgba(255,255,255,.035)'
    ctx.stroke()
  }

  ctx.textAlign = 'left'
  ctx.fillStyle = '#fff'
  ctx.font = '800 31px Arial, sans-serif'
  ctx.fillText(game.title, 28, 48)
  ctx.fillStyle = '#9deee8'
  ctx.font = '600 16px Arial, sans-serif'
  currentIssueLines(draw.issue).forEach((line, index) => ctx.fillText(line, 29, 76 + index * 20, width - 58))

  const balls = draw.numbers
  const ballSize = balls.length > 8 ? 49 : 58
  const gap = 9
  const columns = Math.min(10, balls.length)
  const totalWidth = columns * ballSize + Math.max(0, columns - 1) * gap
  const startX = Math.max(28, (width - totalWidth) / 2)
  balls.forEach((ball, index) => {
    drawBall(ctx, ball, startX + (index % 10) * (ballSize + gap), 104 + headerExtraHeight + Math.floor(index / 10) * 58, ballSize)
  })

  const podium = balls.slice(0, 3)
  if (racingCars) {
    ctx.save()
    ctx.globalAlpha = .94
    ctx.shadowColor = 'rgba(1,25,43,.42)'
    ctx.shadowBlur = 18
    ctx.drawImage(racingCars, 160, 148 + extraHeight, 400, 227)
    ctx.restore()
    const markers = [{ x: 360, y: 302, rank: '1st' }, { x: 225, y: 278, rank: '2nd' }, { x: 495, y: 280, rank: '3rd' }]
    podium.forEach((ball, index) => {
      const marker = markers[index]
      if (marker) drawPodiumMarker(ctx, ball, marker.x, marker.y + extraHeight, lotteryRuleProfile(draw.game_id).family === 'racing' ? marker.rank : `第${index + 1}球`, index === 0)
    })
  } else {
    const podiumLayout = [{ x: 360, y: 244, rank: '1st', size: 82 }, { x: 190, y: 270, rank: '2nd', size: 67 }, { x: 530, y: 278, rank: '3rd', size: 62 }]
    podium.forEach((ball, index) => {
      const slot = podiumLayout[index]
      if (!slot) return
      const slotY = slot.y + extraHeight
      const glow = ctx.createRadialGradient(slot.x, slotY, 8, slot.x, slotY, slot.size)
      glow.addColorStop(0, index === 0 ? 'rgba(255,220,87,.75)' : 'rgba(255,255,255,.42)')
      glow.addColorStop(1, 'rgba(255,255,255,0)')
      ctx.fillStyle = glow
      ctx.beginPath(); ctx.arc(slot.x, slotY, slot.size, 0, Math.PI * 2); ctx.fill()
      ctx.fillStyle = '#fff'
      ctx.textAlign = 'center'
      ctx.font = `900 ${index === 0 ? 48 : 35}px Arial, sans-serif`
      ctx.fillText(String(ball), slot.x, slotY + 8)
      ctx.fillStyle = index === 0 ? '#ffe374' : '#d7f7fa'
      ctx.font = '800 17px Arial, sans-serif'
      ctx.fillText(lotteryRuleProfile(draw.game_id).family === 'racing' ? slot.rank : `第${index + 1}球`, slot.x, slotY - (index === 0 ? 56 : 45))
    })
  }

  const meta = lotteryResultSummary(draw.game_id, balls, game.ruleVersion)
  ctx.fillStyle = 'rgba(4,25,43,.72)'
  ctx.beginPath(); ctx.roundRect(18, 382 + extraHeight, width - 36, 50, 12); ctx.fill()
  ctx.fillStyle = '#8bdfe7'
  ctx.textAlign = 'left'; ctx.font = '600 14px Arial, sans-serif'
  ctx.fillText(meta?.label ?? '开奖号码', 40, 402 + extraHeight)
  ctx.fillStyle = '#fff'; ctx.font = '800 17px Arial, sans-serif'
  ctx.fillText(meta?.spacedText ?? `共 ${balls.length} 个`, 40, 423 + extraHeight)
  if (meta) {
    ctx.fillStyle = '#8bdfe7'; ctx.font = '600 14px Arial, sans-serif'
    ctx.fillText(meta.dragonLabel, 275, 402 + extraHeight)
    ctx.fillStyle = '#fff'; ctx.font = '800 17px Arial, sans-serif'
    ctx.fillText(meta.dragons.join(' '), 275, 423 + extraHeight)
  }
  ctx.fillStyle = '#b8d8df'; ctx.textAlign = 'right'; ctx.font = '600 13px Arial, sans-serif'
  ctx.fillText(new Date(draw.draw_at).toLocaleString('zh-CN', { hour12: false, timeZone: 'Asia/Shanghai' }), 680, 417 + extraHeight, 240)
}

export function paintRecentDrawCard(canvas: HTMLCanvasElement, game: Pick<Game, 'title'> & Partial<Pick<Game, 'ruleVersion'>>, draws: DrawResult[], racingCars: CanvasImageSource | null, pixelRatio = 1) {
  const rows = draws.slice(0, RECENT_DRAW_ROW_LIMIT)
  const { width, height } = recentDrawCardSize(rows)
  // Reserve the same artwork space before and after loading to avoid layout jumps.
  const headerHeight = RECENT_DRAW_HEADER_HEIGHT
  const tableTop = RECENT_DRAW_TABLE_TOP
  const ctx = prepareCanvas(canvas, width, height, pixelRatio)
  if (!ctx) return
  ctx.fillStyle = '#f6fafb'; ctx.fillRect(0, 0, width, height)
  const header = ctx.createLinearGradient(0, 0, width, 0)
  header.addColorStop(0, '#123c52')
  header.addColorStop(1, '#227f8b')
  ctx.fillStyle = header; ctx.fillRect(0, 0, width, headerHeight)
  ctx.fillStyle = '#fff'; ctx.textAlign = 'left'; ctx.font = '800 18px Arial, sans-serif'
  ctx.fillText(`${game.title} · 最近 ${rows.length} 期`, 18, 31)
  if (racingCars) {
    ctx.save()
    ctx.globalAlpha = .96
    ctx.shadowColor = 'rgba(0,18,31,.38)'
    ctx.shadowBlur = 12
    ctx.drawImage(racingCars, width - 157, 1, 145, 82)
    ctx.restore()
  }
  const profile = lotteryRuleProfile(rows[0]?.game_id ?? '')
  const headers: Array<[string, number]> = [['期号', 18], ['号码', 170]]
  if (profile.family !== 'unknown') headers.push([profile.sumLabel, 520], ['龙虎', 625])
  ctx.fillStyle = '#e5f1f4'; ctx.fillRect(0, headerHeight, width, 22)
  ctx.fillStyle = '#567582'; ctx.font = '700 12px Arial, sans-serif'
  headers.forEach(([label, x]) => ctx.fillText(label, x, headerHeight + 15))

  let y = tableTop
  rows.forEach((draw, rowIndex) => {
    const rowHeight = recentRowHeight(draw)
    ctx.fillStyle = rowIndex % 2 ? '#eef5f7' : '#fff'; ctx.fillRect(0, y, width, rowHeight)
    ctx.fillStyle = '#314f5d'; ctx.font = '600 12px Arial, sans-serif'; ctx.textAlign = 'left'
    issueLines(draw.issue).forEach((line, index) => ctx.fillText(line, 18, y + 22 + index * 16, 140))
    draw.numbers.forEach((number, index) => drawBall(ctx, number, 170 + (index % 10) * 30, y + 5 + Math.floor(index / 10) * 30, 25))
    const meta = lotteryResultSummary(draw.game_id, draw.numbers, game.ruleVersion)
    if (meta) {
      ctx.fillStyle = '#405d69'; ctx.font = '700 12px Arial, sans-serif'; ctx.fillText(meta.spacedText, 520, y + 22)
      ctx.fillStyle = '#276c85'; ctx.fillText(meta.dragons.join(' '), 625, y + 22, 80)
    }
    y += rowHeight
  })
}
