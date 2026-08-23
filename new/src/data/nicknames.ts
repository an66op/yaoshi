/** 不关联真实姓名、性别或帐号信息的游戏展示昵称素材库。 */
const adjectives = ['善良的', '安静的', '轻快的', '清醒的', '勇敢的', '自由的', '松弛的', '专注的', '有光的', '可靠的', '温和的', '好奇的', '从容的', '乐观的', '耐心的', '坦然的', '灵巧的', '明亮的', '柔软的', '坚定的', '低调的', '元气的', '随和的', '平和的', '认真地', '慢慢的', '刚好的', '发光的', '热心的', '幸运的']
const objects = ['星云', '云朵', '月光', '晚风', '鲸落', '蒲公英', '朝露', '流星', '纸船', '青柠', '橘子', '银杏', '山谷', '萤火', '雨滴', '白鹭', '候鸟', '海盐', '青山', '小满', '雾岛', '书页', '微光', '平原', '湖面', '原野', '雨林', '树影', '星河', '海浪', '晴空', '松果', '栀子', '风铃', '灯塔', '茶杯', '长椅', '信笺', '北斗', '晨雾']
const symbols = ['⌁', '·', '°', '﹏', '✦', '◌', '〆', 'ᐕ', '☁', '˙']

const sampleNames = [
  '善良的星云', '安静的月光', '轻快的云朵', '清醒的旅程', '勇敢的纸船', '自由的鲸落',
  '有光的山谷', '可靠的微光', '温和的晚风', '好奇的流星', '从容的雨滴', '乐观的青柠',
  '「晨雾」', '⌁海盐', '晴空·07', '雾岛°', '小满﹏', '✦星河', '原野◌', '北斗〆',
]

const pick = <T,>(items: readonly T[]) => items[Math.floor(Math.random() * items.length)]

export function generateNickname() {
  const adjective = pick(adjectives)
  const object = pick(objects)
  const style = Math.floor(Math.random() * 5)
  if (style === 0) return `${adjective}${object}`
  if (style === 1) return `${object}${pick(symbols)}${String(Math.floor(Math.random() * 90) + 10)}`
  if (style === 2) return `「${object}」`
  if (style === 3) return `${pick(symbols)}${adjective}${object}`
  return `${object}${pick(symbols)}`
}

export function nicknameSuggestions(limit = 12) {
  const names = new Set(sampleNames)
  while (names.size < limit) names.add(generateNickname())
  return [...names].slice(0, limit)
}
