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
const digitFiveV3IDs = new Set(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])
const digitThreeIDs = new Set(['official-fc3d', 'official-pl3'])
const pcIDs = new Set(['pc-canada', 'canada-28', 'canada-20'])
const markSixProductContracts = {
  'hong-kong-mark-six': { family: '香港六合彩', version: 'hk-mark6-v1', source: '163 ID18直接提供6个正码和1个特码；仅接受恰好7个互不重复的01–49号码及可信期号、开奖时间。', original: '原版香港六合彩章节包含完整规则与赔率表。' },
  'happy8-mark-six': { family: '快乐8六合彩', version: 'happy8-mark6-v1', source: '163 ID141直接提供6个正码和1个特码；这是163派生7球私盘，不从官方福彩快乐8的20球临时筛选。', original: '原版没有快乐8六合彩独立章节或赔率表。' },
  'new-macau-mark-six': { family: '新澳门六合彩', version: 'new-macau-mark6-v1', source: '163 ID140直接提供6个正码和1个特码；仅当前可信来源版本的7球记录可进入新投注和结算。', original: '原版新澳门六合彩章节包含完整规则与赔率表。' },
  'old-macau-mark-six': { family: '澳门六合彩', version: 'old-macau-mark6-v1', source: '163 ID70直接提供6个正码和1个特码；旧168来源及五位测试记录与当前合同隔离。', original: '原版澳门六合彩章节包含完整规则与赔率表。' },
} as const

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
  family: 'SG时时彩 / 163:64母源＋115校验',
  summary: `${digitFiveV3Profile.summary} 163目录ID64是唯一号码母源，115的sgssc产品只读校验；任何缺期或分歧都会暂停导入、投注及未核验期结算，已保存的可信结果仍可结算，无平台自开回退。`,
  rules: [
    ...digitFiveV3Profile.rules,
    '163目录ID64（站内名称“168SG时时彩”）是唯一号码母源，提供期号、原序五球、开奖时间、有限历史及下一期边界。115的sgssc产品是只读校验源：可否决但不能替代或补写ID64缺失的号码。',
    '最新期号、原序五球和开奖时间、下一期期号和时间，以及最近连续24期必须同时核对通过；任一缺期或分歧都暂停导入、投注及未核验期结算。已保存的可信结果仍可结算，按匹配的注单来源快照幂等处理，不回退为平台自开。',
    '163目录与115均为第三方聚合展示来源；一致不能证明上游独立，也不保证上游独立。163目录ID169属于另一套开奖结果系统，禁止混用、自动备用或接续历史。',
  ],
  differences: [
    ...digitFiveV3Profile.differences,
    { topic: '开奖身份', original: '只有绑定并核验真实SG开奖源，才可宣称与SG外部开奖同步。', current: '当前以163目录ID64为唯一号码母源，115 sgssc只读校验最新期、下一期边界及连续24期；115只能否决，不能补号。异常或分歧时暂停投注及未核验期结算；已保存的可信结果仍可结算。ID169是不同结果系统。', status: 'different' },
  ],
}

const bingoSSCVariant: Record<string, { label: string; range: string }> = {
  'bingo-ssc-1': { label: '一', range: '第1–5个' },
  'bingo-ssc-2': { label: '二', range: '第6–10个' },
  'bingo-ssc-3': { label: '三', range: '第11–15个' },
  'bingo-ssc-4': { label: '四', range: '第16–20个' },
}

const bingoSSCV3ProfileForGame = (gameID: string): CurrentRuleProfile => {
  const variant = bingoSSCVariant[gameID]
  if (!variant) return digitFiveV3Profile
  const platformExtension = variant.label !== '一'
  return {
    ...digitFiveV3Profile,
    family: `宾果时时彩(${variant.label}) / 5球数字彩`,
    summary: `163 ID185按真实开出顺序提供宾果20球，取${variant.range}号码尾数转为第1至第5球；ID135核对同期期号、20球集合和开奖时点，投注与结算使用digits5-v3。${platformExtension ? ` 宾果时时彩(${variant.label})是平台扩展，复用同一规则引擎，不属于原版独立彩种。` : ''}`,
    rules: [
      `开奖转换取ID185原始顺序的${variant.range}号码个位数；不能从按数值排序后的20球反推原始顺序。`,
      'ID185有序母源必须与ID135同期期号、20球集合和开奖时点交叉核对；缺期、重号、越界或集合不一致均不产生可结算开奖。',
      '目录、赔率与助手响应必须同时确认digits5-v3；缺少有效后台赔率的选项保持关闭，不套用默认赔率。',
      ...(platformExtension ? [`宾果时时彩(${variant.label})沿用宾果时时彩(一)的digits5-v3玩法合同，但这是平台扩展，不是原版说明中的独立玩法。`] : []),
      ...digitFiveV3Profile.rules,
    ],
    differences: [
      { topic: '开奖转换', original: variant.label === '一' ? '原版按宾果原始出球顺序取前5号尾数。' : '原版未提供该编号的独立彩种与生产来源合同。', current: `当前仅在163 ID185有序结果与ID135集合交叉核对成功后，取${variant.range}尾数。${platformExtension ? '这是复用digits5-v3的平台扩展，不属于原版。' : ''}`, status: variant.label === '一' ? 'same' : 'current-only' },
      ...digitFiveV3Profile.differences,
    ],
  }
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
  summary: '163 ID185提供宾果20个原始号码的真实开出顺序，ID135核对同期期号、20球集合和开奖时点；再按顺序筛选01–49并取最先符合的7个，前6个为正码、第7个为特码。mark6-v2已覆盖原版全部详细网投市场，聊天投注保持关闭。',
  rules: [
    'ID185有序母源必须与ID135同期期号、20球集合和开奖时点交叉核对；任一侧缺失或不一致时不生成可结算结果。',
    '号码50–80直接跳过；累计7个后停止，不足7个则本期异常且不结算。',
    '已实现特码、两面、总和、色波/半波/半半波、生肖、头尾、五行、正码、正码特、正码1–6及连码等原版详细网投玩法。',
    '生肖按该期开奖日期的农历年动态计算，历史期不会使用今天的生肖表重算。',
    '两面、生肖分组、半波、半半波及正码位置两面遇49按各玩法规则返本；特码半特开49不中奖；纯绿波、生肖、头尾和五行按49正常参与。',
    '特肖、一肖、一尾、总肖（2–7肖及单双）和七色波（红、蓝、绿、和局）均已逐项注册；一肖与一尾使用全部7球，同组重复出现仍只派一次。',
    '合肖按2–11肖分别定价，特码命中所选任一生肖即中且49和局；5–11不中按所选号码数量分别定价。',
    '正肖按前6个正码的命中个数倍增净赢部分，本金只返还一次；结算只使用下注时冻结的有效赔率。',
    '2–5连肖与2–5连尾由公开父玩法接收一张组合注单，并从所选生肖或尾数对应的后台内部定价行中取最低有效赔率；49按原版正常参与。',
    '三中二与二中特都是一次扣款的一张组合票；两档互斥派彩的赔率在下注时同时冻结，不能拆成两张收费注单，也不在结算时重新读取实时赔率。',
    '连肖、连尾、三中二与二中特的组件价格使用“仅后台定价”行；这些行可在管理端配置与查看，但会员不能将其作为独立玩法直接提交。',
    '普通公开玩法使用自身定价行；组合父玩法使用本注所需的全部后台内部定价行。每个必要行都必须显式配置有效赔率、单注最低、单注最高、会员单期和全房单期限额；缺少任一项即关闭该玩法，不使用默认赔率或部分组件兜底。',
  ],
  differences: [
    { topic: '开奖来源', original: '原版六合彩章节以六合彩公司7个号码为开奖；附件也包含宾果衍生彩资料。', current: '当前宾果六合彩明确使用宾果20号筛选01–49、按原顺序取前7个。', status: 'different' },
    { topic: '投注方式', original: '原版以完整网投盘玩法说明及赔率表为主。', current: '当前宾果六合彩只开放详细网投，聊天解析明确关闭。', status: 'different' },
    { topic: '生肖年份', original: '原版以示例生肖年列出固定号码表。', current: '当前按每一期开奖日期所在农历年动态轮换生肖。', status: 'different' },
    { topic: '核心结算', original: '特码、两面、色波、头尾、正码、五行等有明确文字规则。', current: '已按 mark6-v2 实现对应核心规则，并保存规则版本及赔率快照。', status: 'same' },
    { topic: '复杂组合', original: '包含一尾、2–11合肖、正肖、2–5连肖/连尾、三中二、二中特及5–11不中等。', current: 'mark6-v2已完整建模：组合最低价、正肖净赢倍增及双档单票均由服务端冻结价格并结算；内部定价行不能直接下注。', status: 'same' },
    { topic: '赔率与限额', original: '附件包含一份原版赔率表。', current: '本次已依据用户提供的原版资料与参考盘，在后台显式补齐当前运营赔率；普通玩法自身定价行及组合票本注所需的全部内部定价行仍必须完整保存赔率和四类限额，运行时不使用前端默认值或跨彩种借价。', status: 'different' },
  ],
}

const directMarkSixProfileForGame = (gameID: keyof typeof markSixProductContracts): CurrentRuleProfile => {
  const product = markSixProductContracts[gameID]
  return {
    ...bingoMarkSixProfile,
    family: product.family,
    expectedVersion: product.version,
    summary: `${product.source} 前6个号码为正码、第7个为特码；${product.version}复用完整六合彩详细网投核心，但赔率、限额、来源和注单快照均按本彩种独立保存，聊天投注关闭。`,
    rules: [
      product.source,
      '旧的五位模拟记录、未标记来源版本的历史结果及不匹配的待结算测试注单不会进入当前规则合同；不删除历史，但新投注只绑定当前可信版本。',
      ...bingoMarkSixProfile.rules.slice(2),
      '五行使用原版固定号码表，开奖来源不会旋转或替换规则版本中的号码分组。',
    ],
    differences: [
      { topic: '开奖来源', original: product.original, current: product.source, status: gameID === 'happy8-mark-six' ? 'current-only' : 'different' },
      { topic: '投注方式', original: '原版以完整网投盘玩法说明及赔率表为主。', current: `当前只开放${product.version}详细网投，聊天解析关闭。`, status: 'different' },
      ...bingoMarkSixProfile.differences.slice(2).map(item => ({
        ...item,
        current: item.current.replaceAll('mark6-v2', product.version),
      })),
    ],
  }
}

const bingoRacingProfileForGame = (gameID: string): CurrentRuleProfile => {
  const variant = gameID === 'bingo-racing-b' ? 'B' : 'A'
  const range = variant === 'B' ? '后10个' : '前10个'
  const platformExtension = variant === 'B'
  return {
    ...racingProfile,
    family: `宾果赛车(${variant}) / 10名赛车`,
    summary: `163 ID185提供宾果原始20球顺序，ID135核对同期集合和开奖时点；取${range}并在该窗口内按数值排名为1–10，同时保持真实开出顺序。聊天、网投与结算共用racing-v2。${platformExtension ? ' 宾果赛车(B)是平台扩展，不属于原版独立彩种。' : ''}`,
    rules: [
      `ID185有序母源取${range}，ID135核对同期期号、20球集合和开奖时点；任一侧缺失或不一致时不写入可结算结果。`,
      '目录、赔率与助手响应必须同时确认racing-v2；只有后台明确保存有效赔率的选项才能开放。',
      ...(platformExtension ? ['宾果赛车(B)复用宾果赛车(A)的racing-v2玩法合同，但它是平台扩展，不是原版说明中的独立玩法。'] : []),
      `冠亚和大、小、单、双及3–19按${variant}版彩种的选项独立定价；未配置项不会回退到一个通用sum赔率或跨彩种借价。`,
      ...racingProfile.rules,
    ],
    differences: [
      { topic: '开奖映射', original: variant === 'A' ? '原版按宾果原始开奖顺序映射前10球赛车名次。' : '原版未提供B版独立彩种与生产来源合同。', current: `当前由ID185证明原始顺序、ID135核对同期集合，再取${range}排名，未从排序后20球反推顺序。${platformExtension ? 'B版是复用racing-v2的平台扩展，不属于原版。' : ''}`, status: variant === 'A' ? 'same' : 'current-only' },
      ...(variant === 'A'
        ? [{ topic: '冠亚和赔率', original: '原版的大小单双与各和值存在不同赔率。', current: '当前已拆为21个后台定价项，不再共用一个sum赔率。', status: 'same' as const }]
        : [{ topic: '冠亚和赔率', original: '原版未提供B版独立赔率合同。', current: 'B版作为平台扩展采用同形的21个选项级后台定价项，但赔率与限额仍按B版彩种独立配置。', status: 'current-only' as const }]),
      ...racingProfile.differences,
    ],
  }
}

const pcVersionByGame: Record<string, { version: string; label: string; specialRule: string; specialDifference: string }> = {
  'pc-canada': {
    version: 'pc28-v1',
    label: '玩法一',
    specialRule: '当前系统扩展：禁止和值大小/单双反向下注（不含定位两面）；总注大于1时，13/14两面按1.5倍、组合按1倍；总注大于9999时两面按1倍。开13/14时，本期所有下注有效流水为0。',
    specialDifference: 'pc28-v1 额外实现和值两面反向限制、两档13/14两面赔率、组合特别赔率及全期有效流水归零。',
  },
  'canada-28': {
    version: 'pc28-v2',
    label: '玩法二',
    specialRule: '当前系统扩展：禁止和值大小/单双反向下注（不含定位两面）；总注大于1时，13/14两面按1.5倍，总注大于9999时按1倍。总注大于1且开13/14时，组合玩法庄家通吃。',
    specialDifference: 'pc28-v2 额外实现和值两面反向限制、两档13/14两面赔率及总注大于1时的组合庄家通吃。',
  },
  'canada-20': {
    version: 'pc28-v3',
    label: '玩法三',
    specialRule: '当前系统扩展：保持可反向投注；总注大于1时，13/14两面按1.98倍、组合按3.65倍。',
    specialDifference: 'pc28-v3 额外保留可反向投注，并实现13/14两面与组合的独立特别赔率。',
  },
}

const pcProfileForGame = (gameID: string): CurrentRuleProfile => {
  const variant = pcVersionByGame[gameID]
  if (!variant) return pendingProfile('PC / 加拿大28', '原版包含数字、包三、定位、混合、色波，以及玩法一、二、三各自的32项赔率。')
  return {
    family: `PC / 加拿大28 · ${variant.label}`,
    expectedVersion: variant.version,
    modes: '聊天投注 + 详细网投',
    summary: `三球0–9取和值0–27；${gameID === 'pc-canada' ? 'PC加拿大' : gameID === 'canada-28' ? '加拿大28' : '加拿大2.0'}固定绑定${variant.label}，下注与结算保存${variant.version}规则版本。`,
    rules: [
      '数字玩法支持和值0–27；特码包三必须选择三个互不相同的和值号码。单点数字每期最多10个不同点数。',
      '定位玩法支持第1–3球号码0–9及大、小、单、双；龙虎和比较第一球与第三球。',
      '混合玩法支持和值大、小、单、双、组合、极大、极小、豹子、对子、顺子及色波；原版明确890和901算顺子。',
      '聊天解析与详细网投共用同一玩法目录；赔率只读取后台当前彩种配置，注单保存下注时赔率快照。',
      variant.specialRule,
    ],
    differences: [
      { topic: '版本绑定', original: '原版分别列出玩法一、玩法二、玩法三。', current: `${gameID} 固定绑定${variant.label}（${variant.version}），不同版本不得互相结算。`, status: 'same' },
      { topic: '共同玩法', original: '原版包含数字、包三、三球定位、龙虎和、混合、色波及梭哈。', current: '当前聊天和详细网投使用同一套原子玩法、选择范围与服务端结算。', status: 'same' },
      { topic: '顺子边界', original: '原版明确8、9、0与9、0、1算顺子。', current: 'PC28专用形态判定已按循环顺子处理；同一组三个号码不受排列顺序影响。', status: 'same' },
      { topic: '13/14规则', original: `所给原版${variant.label}章节没有13/14动态降赔、通吃或有效流水条款。`, current: variant.specialDifference, status: 'current-only' },
      { topic: '赔率来源', original: '原版文档附有参考赔率。', current: '当前赔率取后台按彩种保存的配置；未配置玩法保持禁用，不使用默认赔率。', status: 'different' },
    ],
  }
}

export function currentRuleProfileForGame(game: Pick<AdminGame, 'id' | 'name' | 'rules_ready' | 'rule_version' | 'rules_message'> & Partial<Pick<AdminGame, 'source_kind' | 'source_name'>>): CurrentRuleProfile {
  if (game.id === 'bingo-racing-a' || game.id === 'bingo-racing-b') return bingoRacingProfileForGame(game.id)
  if (racingIDs.has(game.id)) return racingProfile
  if (digitFiveV3IDs.has(game.id)) return game.id.startsWith('bingo-ssc-') ? bingoSSCV3ProfileForGame(game.id) : game.id === 'sg-ssc' ? sgSSCV3Profile : digitFiveV3Profile
  if (digitThreeIDs.has(game.id)) return digitThreeProfile
  if (game.id === 'bingo-mark-six') return bingoMarkSixProfile
  if (pcIDs.has(game.id)) return pcProfileForGame(game.id)
  if (game.id in markSixProductContracts) return directMarkSixProfileForGame(game.id as keyof typeof markSixProductContracts)
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
