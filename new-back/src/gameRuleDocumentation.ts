import type { AdminGame } from './api'

export type RuleDifferenceStatus = 'same' | 'different' | 'current-only' | 'pending'

export type RuleDifference = {
  topic: string
  original: string
  current: string
  status: RuleDifferenceStatus
}

export type CurrentRuleProfile = {
  family: string
  expectedVersion: string
  modes: string
  summary: string
  rules: string[]
  differences: RuleDifference[]
}

export type OriginalRuleSection = {
  id: string
  title: string
  content: string
  rulesContent: string
  odds: OriginalRuleOdds[]
  startLine: number
  endLine: number
}

export type OriginalRuleOdds = {
  play: string
  multiplier: string
}

type OriginalGameSectionSpec = {
  title: string
  heading: RegExp
  rewindTo?: RegExp
}

// The supplied original is one exported manual containing rules and odds for
// several concrete games. Only some chapters use the “游戏规则 …” prefix, so
// treating that prefix as the index loses most of the games. Keep the order of
// the source and match the first heading belonging to every named game/version.
const originalGameSectionSpecs: OriginalGameSectionSpec[] = [
  { title: '极速赛车', heading: /^游戏规则\s+极速赛车\s*$/ },
  { title: '香港六合彩', heading: /^游戏规则\s+香港六合彩\s*$/ },
  { title: '宾果赛车(A)', heading: /^宾果赛车\(A\)\s*$/ },
  { title: '宾果时时彩(一)', heading: /^宾果时时彩\(一\)\s*$/ },
  { title: '宾果六合彩', heading: /^宾果六合彩\s*$/ },
  { title: '澳洲幸运10', heading: /^游戏规则\s+澳洲幸运10\s*$/ },
  { title: '澳洲幸运5', heading: /^游戏规则\s+澳洲幸运5\s*$/ },
  { title: '幸运飞艇', heading: /^幸运飞艇\s*$/ },
  { title: '极速飞艇', heading: /^游戏规则\s+极速飞艇\s*$/ },
  // This export omitted the game name before its rule text and only printed it
  // above the odds table. Rewind to the preceding “投注方式” to retain its rules.
  { title: '极速时时彩', heading: /^极速时时彩\s*$/, rewindTo: /^投注方式\s*$/ },
  { title: 'SG飞艇', heading: /^SG飞艇\s*$/ },
  { title: '幸运时时彩', heading: /^游戏规则\s+幸运时时彩\s*$/ },
  { title: 'PC蛋蛋', heading: /^PC蛋蛋\s*$/ },
  { title: '极速快3', heading: /^极速快3\s*$/ },
  { title: '加拿大28-玩法一', heading: /^游戏规则\s+加拿大28-玩法一\s*$/ },
  { title: '加拿大28-玩法二', heading: /^加拿大28-玩法二\s*$/ },
  { title: '加拿大28-玩法三', heading: /^加拿大28-玩法三\s*$/ },
  { title: '动物运动会', heading: /^动物运动会\s*$/ },
  { title: '五分运动会', heading: /^五分运动会\s*$/ },
  { title: '福彩3D', heading: /^游戏规则\s+福彩3D\s*$/ },
  { title: '体彩排列3', heading: /^体彩排列3\s*$/ },
  { title: '澳门六合彩', heading: /^游戏规则\s+澳门六合彩\s*$/ },
  { title: '新澳门六合彩', heading: /^新澳门六合彩\s*$/ },
]

const racingIDs = new Set(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10'])
const digitFiveV3IDs = new Set(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1'])
const unverifiedBingoIDs = new Set(['bingo-racing-b', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])
const digitThreeIDs = new Set(['official-fc3d', 'official-pl3'])
const pcIDs = new Set(['pc-canada', 'canada-28', 'canada-20'])
const markSixReferenceIDs = new Set(['hong-kong-mark-six', 'happy8-mark-six', 'new-macau-mark-six', 'old-macau-mark-six'])

const pendingProfile = (family: string, original: string): CurrentRuleProfile => ({
  family,
  expectedVersion: '未绑定',
  modes: '不受理投注',
  summary: '当前仅保留彩种、开奖或展示入口，尚未形成解析、赔率、限额和结算一致的规则版本。',
  rules: [
    '没有规则版本时，聊天、网投、后台代投和结算入口都不应接受新注单。',
    '不能按名称、号码数量或同类彩种推断规则；后续开放必须建立新的版本化规则合同。',
  ],
  differences: [{
    topic: '开放状态',
    original,
    current: '当前规则未绑定，系统不受理投注。',
    status: 'pending',
  }],
})

const racingProfile: CurrentRuleProfile = {
  family: '10名赛车 / 飞艇',
  expectedVersion: 'racing-v2',
  modes: '聊天投注 + 详细网投',
  summary: '10个互不重复号码，支持名次号码、大小单双、1–5名龙虎及冠亚和；每个展开项独立计费和结算。',
  rules: [
    '第1–10名号码范围为1–10；位置10和号码10在聊天指令中可以用0表示。',
    '大小单双支持第1–10名；龙虎仅第1–5名，依次与第10–6名镜像比较。',
    '冠亚和使用前两名和值，可投3–19以及大、小、单、双。',
    '未写位置的聊天指令默认冠军；前三、后三、前五、后五按位置展开。',
    '梭哈按展开后的投注项等额分配余额中的最大整数金额，余数及小数积分保留。',
  ],
  differences: [
    { topic: '主要玩法', original: '原版包含名次号码、两面、龙虎、冠亚和及紧凑聊天指令。', current: '当前 racing-v2 保留这些核心含义，并同时提供详细网投。', status: 'same' },
    { topic: '赔率来源', original: '原版文档附有一组固定参考赔率。', current: '当前赔率取后台按彩种保存的实时配置，历史注单保留下注时赔率快照。', status: 'different' },
    { topic: '规则边界', original: '原版以文字和输入示例描述识别方式。', current: '当前新增服务端玩法编号、号码范围、位置范围及结算版本校验。', status: 'current-only' },
  ],
}

const digitFiveV3Profile: CurrentRuleProfile = {
  family: '5球时时彩 / 幸运5（完整三段形态）',
  expectedVersion: 'digits5-v3',
  modes: '聊天投注 + 详细网投',
  summary: '5球、每球0–9，固定使用 digits5-v3；完整支持前三、中三、后三形态，以及第一球对第五球的龙、虎、和。',
  rules: [
    '第1–5球可投0–9及大、小、单、双；0–4为小，5–9为大。',
    '不写球位时，号码或大小单双会展开到第1–5球，每个球位与选号独立计注。',
    '前三使用第1–3球，中三使用第2–4球，后三使用第3–5球；均可投豹子、顺子、对子、半顺、杂六。',
    '聊天指令写明前三、中三或后三时只投对应窗口；未写窗口的形态指令会展开为三个独立注单。',
    '龙虎和比较第一球与第五球：大于为龙、小于为虎、相同为和；“和”使用独立后台赔率。',
    '原版未定义890、901边界；当前规则固定沿用循环顺子判定，未套用PC/加拿大28的排除条款。',
    '总和、总和尾及第二球对第四球龙虎不属于当前规则合同，不开放投注。',
    '彩种必须精确绑定 digits5-v3 且规则就绪；缺失或不匹配的版本不受理投注。',
  ],
  differences: [
    { topic: '球位号码与两面', original: '原版支持1–5球号码、大小单双以及不写球位的展开写法。', current: '当前核心球位玩法一致，并增加服务端位置和号码校验。', status: 'same' },
    { topic: '形态位置', original: '原版写有前三、中三、后三形态；未写位置的形态同时展开三段。', current: 'digits5-v3 已按三个滑动窗口独立下注和结算，聊天与详细网投使用同一合同。', status: 'same' },
    { topic: '龙虎和', original: '原版以第一球和第五球比较，可投龙、虎、和。', current: 'digits5-v3 使用同一比较关系，并为“和”设置独立玩法编号和后台赔率。', status: 'same' },
    { topic: '本地附加玩法', original: '所给5球说明没有总和、总和尾或第二球对第四球龙虎。', current: 'digits5-v3 投注目录、聊天解析和详细网投均不开放这些附加玩法。', status: 'same' },
    { topic: '顺子边界', original: '极速时时彩与澳洲幸运5章节未说明890、901是否为顺子。', current: '当前明确固定为循环顺子；这是本地边界，不引用PC/加拿大28章节的排除规则。', status: 'current-only' },
    { topic: '赔率来源', original: '原版文档附固定参考赔率。', current: '当前以后台彩种赔率配置及注单赔率快照为准。', status: 'different' },
  ],
}

const sgSSCV3Profile: CurrentRuleProfile = {
  ...digitFiveV3Profile,
  family: 'SG时时彩 / 王者平台自开',
  summary: `${digitFiveV3Profile.summary} 当前开奖为王者平台自开（王者开奖），不宣称与SG外部开奖同步。`,
  rules: [...digitFiveV3Profile.rules, 'SG时时彩的玩法合同已完整接入；开奖身份为王者平台自开，不将彩种名称或来源标签当作外部同步凭据。'],
  differences: [
    ...digitFiveV3Profile.differences,
    { topic: '开奖身份', original: '只有绑定并核验真实SG开奖源，才可宣称与SG外部开奖同步。', current: '当前产品使用王者平台自开（王者开奖）；完整玩法接入不代表外部开奖同步。', status: 'different' },
  ],
}

const bingoSSC1V3Profile: CurrentRuleProfile = {
  ...digitFiveV3Profile,
  family: '宾果时时彩(一) / 5球数字彩',
  summary: '按真实开出顺序取宾果原始20号的前5个号码尾数，转为第1至第5球；投注与结算使用digits5-v3。',
  rules: [
    '开奖转换取原始顺序前5号的个位数；不能从按数值排序后的20号数组反推原始顺序。',
    '原始顺序源必须与宾果官方期号及20个号码集合交叉核对；缺期、重号、越界或集合不一致均不产生可结算开奖。',
    ...digitFiveV3Profile.rules,
  ],
  differences: [
    { topic: '开奖转换', original: '原版按宾果原始出球顺序取前5号尾数。', current: '当前仅在原始顺序源与官方20号结果交叉核对成功后接受该期。', status: 'same' },
    ...digitFiveV3Profile.differences,
  ],
}

const digitThreeProfile: CurrentRuleProfile = {
  family: '3球数字彩',
  expectedVersion: 'digits3-v2',
  modes: '彩种开放后支持聊天投注 + 详细网投',
  summary: '3球、每球0–9；支持号码、两面、总和、总和尾、第一球龙虎及前三形态。',
  rules: [
    '第1–3球可投0–9及大、小、单、双；0–4为小，5–9为大。',
    '三球总和14及以上为大；总和尾为和值个位0–9。',
    '第一球与第三球比较产生龙、虎或和；当前下注选项只开放龙、虎。',
    '豹子、顺子、对子、半顺、杂六使用全部三球判断。',
  ],
  differences: [
    { topic: '玩法范围', original: '附件中的福彩3D与体彩排列3包含一字、二字、三字定位/组合、质合、跨度、组选三、组选六等完整私盘玩法。', current: '当前 digits3-v2 仅实现通用三球号码、两面、总和/尾、龙虎及三球形态。', status: 'different' },
    { topic: '开放状态', original: '附件提供完整规则和赔率表。', current: '两个官方三位彩虽然有规则合同，但种子目录默认关闭；实际开放还受彩种开关与来源状态控制。', status: 'different' },
  ],
}

const bingoMarkSixProfile: CurrentRuleProfile = {
  family: '宾果六合彩',
  expectedVersion: 'mark6-v2',
  modes: '仅详细网投',
  summary: '从宾果20个原始号码中按开出顺序筛选01–49，取最先符合的7个；前6个为正码，第7个为特码。',
  rules: [
    '号码50–80直接跳过；累计7个后停止，不足7个则本期异常且不结算。',
    '已实现特码、两面、总和、色波/半波/半半波、生肖、头尾、五行、正码、正码特及正码1–6核心玩法。',
    '生肖按该期开奖日期的农历年动态计算，历史期不会使用今天的生肖表重算。',
    '两面、生肖分组、半波、半半波及正码位置两面遇49按各玩法规则返本；特码半特开49不中奖；纯绿波、生肖、头尾和五行按49正常参与。',
    '四全中、三全中、二全中、特串和五不中已经有单档结算；服务端玩法目录会显示其实际开放状态。',
    '具有两档赔率、按命中数倍增或组合最低赔率的复杂玩法，在模型完整前保持不可提交。',
  ],
  differences: [
    { topic: '开奖来源', original: '原版六合彩章节以六合彩公司7个号码为开奖；附件也包含宾果衍生彩资料。', current: '当前宾果六合彩明确使用宾果20号筛选01–49、按原顺序取前7个。', status: 'different' },
    { topic: '投注方式', original: '原版以完整网投盘玩法说明及赔率表为主。', current: '当前宾果六合彩只开放详细网投，聊天解析明确关闭。', status: 'different' },
    { topic: '生肖年份', original: '原版以示例生肖年列出固定号码表。', current: '当前按每一期开奖日期所在农历年动态轮换生肖。', status: 'different' },
    { topic: '核心结算', original: '特码、两面、色波、头尾、正码、五行等有明确文字规则。', current: '已按 mark6-v2 实现对应核心规则，并保存规则版本及赔率快照。', status: 'same' },
    { topic: '复杂组合', original: '包含三中二、二中特、正肖倍增、合肖、自选不中、连肖、连尾等。', current: '双赔率、倍增和组合最低赔率尚未全部开放，界面会逐项标记。', status: 'pending' },
    { topic: '赔率来源', original: '附件包含一份原版赔率表。', current: '当前不复制原版赔率；只使用后台已配置赔率，未配置项保持关闭。', status: 'different' },
  ],
}

const bingoRacingAProfile: CurrentRuleProfile = {
  ...pendingProfile('宾果赛车(A)', '原版描述了由宾果开奖结果映射10名赛车的规则。'),
  differences: [
    { topic: '开奖映射', original: '原版按宾果原始开奖顺序映射赛车名次。', current: '当前来源顺序与目标定义尚未完成一致性核验，因此未绑定赛车规则。', status: 'pending' },
    { topic: '开放状态', original: '原版提供赛车投注说明。', current: '当前展示开奖与说明，不受理新注单。', status: 'different' },
  ],
}

const bingoRacingAProfileForGame = (game: Pick<AdminGame, 'rules_ready' | 'rule_version'>): CurrentRuleProfile => {
  if (!game.rules_ready || game.rule_version !== 'racing-v2') return bingoRacingAProfile
  return {
    ...racingProfile,
    family: '宾果赛车(A) / 10名赛车',
    summary: '宾果原始出球顺序已经服务端审计并转换为10个互不重复名次；聊天、网投与结算共用racing-v2合同。',
    rules: [
      '只有开奖源能提供可审计的真实开出顺序时，服务端才会返回racing-v2且标记规则就绪。',
      '冠亚和大、小、单、双及3–19按选项独立定价；只有后台明确保存过的赔率才能开放对应选项。',
      ...racingProfile.rules,
    ],
    differences: [
      { topic: '开奖映射', original: '原版按宾果原始开奖顺序映射10名赛车。', current: '当前来源转换已通过服务端审计，且未从排序后号码反推出球顺序。', status: 'same' },
      { topic: '冠亚和赔率', original: '原版的大小单双与各和值存在不同赔率。', current: '当前已拆为21个后台定价项，不再共用一个sum赔率。', status: 'same' },
      ...racingProfile.differences,
    ],
  }
}

const pcVersionByGame: Record<string, { version: string; label: string; specialRule: string; specialDifference: string }> = {
  'pc-canada': {
    version: 'pc28-v1',
    label: '玩法一',
    specialRule: '禁止和值大小/单双反向下注（不含定位两面）；总注大于1时，13/14两面按1.5倍、组合按1倍；总注大于9999时两面按1倍。开13/14时，本期所有下注有效流水为0。',
    specialDifference: 'pc28-v1 已实现和值两面反向限制、两档13/14两面赔率、组合特别赔率及全期有效流水归零。',
  },
  'canada-28': {
    version: 'pc28-v2',
    label: '玩法二',
    specialRule: '禁止和值大小/单双反向下注（不含定位两面）；总注大于1时，13/14两面按1.5倍，总注大于9999时按1倍。总注大于1且开13/14时，组合玩法庄家通吃。',
    specialDifference: 'pc28-v2 已实现和值两面反向限制、两档13/14两面赔率及总注大于1时的组合庄家通吃。',
  },
  'canada-20': {
    version: 'pc28-v3',
    label: '玩法三',
    specialRule: '原版未规定反向限制；总注大于1时，13/14两面按1.98倍、组合按3.65倍。',
    specialDifference: 'pc28-v3 保留可反向投注，并实现13/14两面与组合的独立特别赔率。',
  },
}

const pcProfileForGame = (gameID: string): CurrentRuleProfile => {
  const variant = pcVersionByGame[gameID]
  if (!variant) return pendingProfile('PC / 加拿大28', '原版包含数字、包三、定位、混合、色波，以及玩法一、二、三的13/14特别条款。')
  return {
    family: `PC / 加拿大28 · ${variant.label}`,
    expectedVersion: variant.version,
    modes: '聊天投注 + 详细网投',
    summary: `三球0–9取和值0–27；${gameID === 'pc-canada' ? 'PC加拿大' : gameID === 'canada-28' ? '加拿大28' : '加拿大2.0'}固定绑定${variant.label}，下注与结算保存${variant.version}规则版本。`,
    rules: [
      '数字玩法支持和值0–27；特码包三必须选择三个互不相同的和值号码。单点数字每期最多10个不同点数。',
      '定位玩法支持第1–3球号码0–9及大、小、单、双；龙虎和比较第一球与第三球。',
      '混合玩法支持和值大、小、单、双、组合、极大、极小、豹子、对子、顺子及色波；890和901不算顺子。',
      '聊天解析与详细网投共用同一玩法目录；赔率只读取后台当前彩种配置，注单保存下注时赔率快照。',
      variant.specialRule,
    ],
    differences: [
      { topic: '版本绑定', original: '原版分别列出玩法一、玩法二、玩法三。', current: `${gameID} 固定绑定${variant.label}（${variant.version}），不同版本不得互相结算。`, status: 'same' },
      { topic: '共同玩法', original: '原版包含数字、包三、三球定位、龙虎和、混合、色波及梭哈。', current: '当前聊天和详细网投使用同一套原子玩法、选择范围与服务端结算。', status: 'same' },
      { topic: '顺子边界', original: '当前本地规则明确8、9、0与9、0、1不算顺子。', current: 'PC28专用形态判定使用相同排除条款，不套用时时彩循环顺子。', status: 'same' },
      { topic: '13/14规则', original: `原版${variant.label}规定独立的13/14特别条款。`, current: variant.specialDifference, status: 'same' },
      { topic: '赔率来源', original: '原版文档附有参考赔率。', current: '当前赔率取后台按彩种保存的配置；未配置玩法保持禁用，不使用默认赔率。', status: 'different' },
    ],
  }
}

export function currentRuleProfileForGame(game: Pick<AdminGame, 'id' | 'name' | 'rules_ready' | 'rule_version' | 'rules_message'> & Partial<Pick<AdminGame, 'source_kind' | 'source_name'>>): CurrentRuleProfile {
  if (racingIDs.has(game.id)) return racingProfile
  if (digitFiveV3IDs.has(game.id)) return game.id === 'bingo-ssc-1' ? bingoSSC1V3Profile : game.id === 'sg-ssc' ? sgSSCV3Profile : digitFiveV3Profile
  if (unverifiedBingoIDs.has(game.id)) return {
    ...pendingProfile(game.name || '宾果待核验彩种', '宾果变体必须各自核验开奖映射与玩法合同，不能套用宾果赛车(A)或宾果时时彩(一)。'),
    modes: '仅展示 · 不受理投注',
    summary: '该宾果变体的开奖映射与玩法合同尚待核验。当前仅展示彩种与开奖结果，不绑定可下注规则。',
  }
  if (digitThreeIDs.has(game.id)) return digitThreeProfile
  if (game.id === 'bingo-mark-six') return bingoMarkSixProfile
  if (game.id === 'bingo-racing-a') return bingoRacingAProfileForGame(game)
  if (pcIDs.has(game.id)) return pcProfileForGame(game.id)
  if (markSixReferenceIDs.has(game.id)) return pendingProfile('六合彩参考彩种', '原版包含六合彩完整玩法与赔率表。')
  return pendingProfile(game.name || '未分类彩种', '原版是否包含对应章节，可在“原版说明”中按名称搜索。')
}

/** Documentation uses the same exact-version, fail-closed boundary as betting. */
export function currentRuleBindingReady(game: Pick<AdminGame, 'id' | 'name' | 'rules_ready' | 'rule_version' | 'rules_message'> & Partial<Pick<AdminGame, 'source_kind' | 'source_name'>>) {
  const expected = currentRuleProfileForGame(game).expectedVersion
  return game.rules_ready === true && expected !== '未绑定' && game.rule_version === expected
}

export function parseOriginalRuleDocument(source: string): OriginalRuleSection[] {
  const normalized = source.replace(/\r\n?/g, '\n')
  const split = normalized.split('\n')
  const lines = split.at(-1) === '' ? split.slice(0, -1) : split
  const concreteGames: Array<{ index: number; title: string }> = []
  let cursor = 0
  for (const spec of originalGameSectionSpecs) {
    const headingIndex = lines.findIndex((line, index) => index >= cursor && spec.heading.test(line.trim()))
    if (headingIndex < 0) {
      concreteGames.length = 0
      break
    }
    let startIndex = headingIndex
    if (spec.rewindTo) {
      for (let index = headingIndex - 1; index >= cursor; index -= 1) {
        if (spec.rewindTo.test(lines[index].trim())) {
          startIndex = index
          break
        }
      }
    }
    concreteGames.push({ index: startIndex, title: spec.title })
    cursor = headingIndex + 1
  }

  const headings = concreteGames.length === originalGameSectionSpecs.length
    ? concreteGames
    : lines.flatMap((line, index) => {
      const match = line.trim().match(/^游戏规则\s+(.+?)\s*$/)
      return match?.[1] ? [{ index, title: match[1].trim() }] : []
    })
  return headings.map((heading, index) => {
    const end = headings[index + 1]?.index ?? lines.length
    const sectionLines = lines.slice(heading.index, end)
    const oddsHeadingIndex = sectionLines.findIndex((line, lineIndex) => (
      line.trim() === heading.title
      && sectionLines[lineIndex + 1]?.trim() === '玩法'
      && sectionLines[lineIndex + 2]?.trim() === '赔率'
    ))
    const oddsLines = oddsHeadingIndex >= 0
      ? sectionLines.slice(oddsHeadingIndex + 3).map(line => line.trim()).filter(Boolean)
      : []
    const odds: OriginalRuleOdds[] = []
    for (let lineIndex = 0; lineIndex < oddsLines.length; lineIndex += 2) {
      odds.push({ play: oddsLines[lineIndex], multiplier: oddsLines[lineIndex + 1] || '—' })
    }
    return {
      id: `${index + 1}-${heading.title}`,
      title: heading.title,
      content: sectionLines.join('\n').trimEnd(),
      rulesContent: (oddsHeadingIndex >= 0 ? sectionLines.slice(0, oddsHeadingIndex) : sectionLines).join('\n').trimEnd(),
      odds,
      startLine: heading.index + 1,
      endLine: end,
    }
  })
}

export function originalRuleDocumentLineCount(source: string): number {
  if (!source) return 0
  const normalized = source.replace(/\r\n?/g, '\n')
  const lines = normalized.split('\n')
  return lines.at(-1) === '' ? lines.length - 1 : lines.length
}

export const differenceStatusLabel: Record<RuleDifferenceStatus, string> = {
  same: '主要含义一致',
  different: '存在差异',
  'current-only': '当前新增',
  pending: '尚未对齐',
}

export const originalNamedGameCount = originalGameSectionSpecs.length
