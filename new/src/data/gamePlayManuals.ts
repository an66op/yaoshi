import type { Game } from '../types'
import { gameRulesReady, isDigit5V3Game, isPC28RuleVersion, pc28RuleVersionForGame } from '../utils/lotteryRules'

export type GameManualSection = {
  title: string
  body: string
  examples?: string[]
}

export type GameManual = {
  id: string
  title: string
  gameId?: string
  status: 'implemented' | 'partial' | 'reference'
  statusText: string
  betModes: { chat: boolean; web: boolean }
  summary: string
  sourceURL?: string
  sections: GameManualSection[]
  auditNotes?: string[]
}

const racingIDs = new Set(['speed-racing', 'speed-fly', 'sg-fly', 'fly-racing', 'au-lucky-10', 'bingo-racing-a', 'bingo-racing-b'])
const digitFiveIDs = new Set(['speed-ssc', 'sg-ssc', 'au-lucky-5', 'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4'])
const digitThreeIDs = new Set(['official-fc3d', 'official-pl3'])
const pc28IDs = new Set(['pc-canada', 'canada-28', 'canada-20'])
const markSixProducts = {
  'bingo-mark-six': {
    version: 'mark6-v2',
    summary: '每期从宾果20个原始号码中按开出顺序筛选01–49，取最先符合的7个号码',
    source: '163 ID185提供宾果每5分钟开出的20球真实顺序，ID135核对同期期号、20球集合和开奖时点；任一侧缺失或不一致时不生成可结算结果。营业时段为07:05–23:55、每日203期。系统从ID185原序只采用01–49；遇到50–80直接跳过，累计取得7个后停止。若整期不足7个合格号码，本期视为异常，不开奖也不结算。',
    example: '原始：52、35、71、34、23、30、22、06、20… → 开奖：35、34、23、30、22、06 + 20',
  },
  'hong-kong-mark-six': {
    version: 'hk-mark6-v1',
    summary: '163 ID18直接提供7个互不重复的01–49号码',
    source: '163 ID18是本彩种的唯一开奖母源，直接提供期号、6个正码、1个特码及下一期开奖时间。只有恰好7个互不重复的01–49号码、合法期号和可验证开奖时间才会写入；旧的测试号码和未标记来源版本的历史记录不会进入新投注或新结算。',
    example: '直接开奖：30、04、11、26、08、07 + 42',
  },
  'happy8-mark-six': {
    version: 'happy8-mark6-v1',
    summary: '163 ID141直接提供派生的7个互不重复01–49号码',
    source: '163 ID141是本彩种的唯一开奖母源，直接提供6个正码和1个特码；它是163派生的7球私盘产品，并非从官方福彩快乐8的20球结果临时筛选，也不宣称等同官方福彩快乐8。只有完整、互不重复且均为01–49的结果才会进入投注和结算。',
    example: '直接开奖：15、25、31、24、19、37 + 29',
  },
  'new-macau-mark-six': {
    version: 'new-macau-mark6-v1',
    summary: '163 ID140直接提供7个互不重复的01–49号码',
    source: '163 ID140是本彩种的唯一开奖母源，直接提供期号、6个正码、1个特码及下一期开奖时间。只有来源版本、号码形状与时间边界全部通过校验的期次才允许下注和结算；旧的五位测试记录会被隔离。',
    example: '直接开奖：22、24、19、10、20、01 + 30',
  },
  'old-macau-mark-six': {
    version: 'old-macau-mark6-v1',
    summary: '163 ID70直接提供7个互不重复的01–49号码',
    source: '163 ID70是本彩种的唯一开奖母源，直接提供期号、6个正码、1个特码及下一期开奖时间。旧168来源与五位测试记录不会混入新投注；只有带当前可信来源版本的7球结果才允许结算。',
    example: '前6个号码为正码，第7个号码为特码',
  },
} as const
const markSixIDs = new Set(Object.keys(markSixProducts))

const racingSections: GameManualSection[] = [
  {
    title: '名次、号码与两面',
    body: '第1至第10名可投号码1–10及大、小、单、双；龙、虎仅可投第1至第5名，并与第10至第6名逐一镜像比较。号码或位置10可用0表示。未写位置时默认只投冠军。',
    examples: ['3/大/5 → 第三名 大-5', '123大/5 → 冠军 1、2、3、大，各5', '0/1/5 → 第十名 1-5', '10大5 → 冠军与第十名 大，各5'],
  },
  {
    title: '紧凑写法',
    body: '非数字玩法可省略玩法与金额之间的斜杠；纯数字玩法仍必须保留斜杠，避免无法区分号码与金额。',
    examples: ['1大5 = 1/大/5', '大单/5 → 冠军 大、单，各5'],
  },
  {
    title: '冠亚和',
    body: '“和”是冠亚和别名。可投准确和值3–19或大、小、单、双；和后连续写3–9时按多个单个和值展开，10–19请写完整和值。',
    examples: ['和/大/5 → 冠亚和大-5', '和/345/5 或 和345/5 → 和值3、4、5，各5', '冠亚/14/5 → 和值14-5'],
  },
  {
    title: '位置组',
    body: '前三、后三、前五、后五按字面展开位置。每个位置与每个选号形成独立注单。',
    examples: ['前三/2/5 → 第1、2、3名号码2，各5', '后五/大/5 → 第6至第10名大，各5'],
  },
  {
    title: '梭哈',
    body: '梭哈必须作为一条独立聊天指令提交；系统以余额可分配的最大整数金额，对展开后的每个投注项等额下注。不能整除的积分及小数余额会保留，详细网投面板不提供梭哈按钮。',
    examples: ['余额100，1/123/梭哈 → 每项33，余额1', '余额100.5，大单梭哈 → 每项50，余额0.5'],
  },
]

const digitFiveSections: GameManualSection[] = [
  {
    title: '第1至第5球',
    body: '每球可投号码0–9及大、小、单、双。明确写球位时只投该球；非数字玩法可省略最后一个斜杠。',
    examples: ['1/1大/20 → 第一球号码1与大，各20', '1大5 = 1/大/5'],
  },
  {
    title: '不定位买法',
    body: '不写球位时，号码或大小单双会同时应用到第1至第5球。每个球位、每个选号分别计为一注。',
    examples: ['大/20 → 第1至第5球大，各20', '12/5 → 第1至第5球号码1、2，各5'],
  },
  {
    title: '前三、中三、后三形态',
    body: '支持豹子、顺子、对子、半顺、杂六，可写“前三/豹子/5”、 “中三顺子/5”或“豹子5”。未写位置的形态按前三、中三、后三展开。',
    examples: ['前三豹子5', '中三/顺子/5', '豹子5 → 前三、中三、后三豹子，各5'],
  },
  {
    title: '梭哈',
    body: '梭哈必须作为一条独立聊天指令提交；按展开后的所有投注项，以最大整数金额等额下注，余数和小数余额保留。',
    examples: ['余额100.5，大梭哈 → 第1至第5球大，各20，余额0.5'],
  },
]

const digitFiveV3Sections: GameManualSection[] = [
  ...digitFiveSections,
  {
    title: '龙虎和',
    body: '仅比较第一球与第五球：第一球大于第五球为龙，小于为虎，相同为和。龙、虎共用龙虎赔率，和使用后台单独配置的赔率；没有有效赔率时对应选项不可投注。',
    examples: ['1/龙/5', '1/虎/5', '1/和/5 → 第一球与第五球相同则中奖'],
  },
]

const digitThreeSections: GameManualSection[] = [
  {
    title: '第1至第3球',
    body: '每球可投号码0–9及大、小、单、双；不写球位时，号码或大小单双同时应用到三球。龙虎仅开放第一球与第三球比较，相同为和但当前只接受龙、虎投注。',
    examples: ['1/09/20 → 第一球号码0、9，各20', '大/20 → 第1至第3球大，各20', '1/龙/20 → 第一球龙-20'],
  },
  {
    title: '总和与前三形态',
    body: '三球总和可投大、小、单、双；总和尾可投0–9。豹子、顺子、对子、半顺、杂六按全部三球判断。',
    examples: ['总和/大/20', '总和尾/7/20', '豹子20 或 前三/豹子/20'],
  },
  {
    title: '梭哈',
    body: '梭哈必须作为一条独立聊天指令提交，按展开后的投注项以最大整数金额等额下注；不能整除的积分与小数余额保留。',
    examples: ['余额100，1/123/梭哈 → 每项33，余额1'],
  },
]

const pc28Sections: GameManualSection[] = [
  {
    title: '数字与特码包三',
    body: '数字范围0–27，数字玩法必须带斜杠。特码包三须从0–27选择三个互不相同号码。',
    examples: ['1/5#2/5 → 号码1、2，各5', '特码/1/2/3/5 → 包三1、2、3，金额5'],
  },
  {
    title: '三球定位',
    body: '球位范围1–3、号码范围0–9，每球可投号码与大、小、单、双；定位玩法通常使用两个斜杠，定位两面可用紧凑写法。第一球与第三球比较产生龙、虎、和，“和”使用独立赔率。',
    examples: ['1/1/5 → 第一球号码1-5', '13/89/5 → 第一、三球号码8、9，各5', '1大5 → 第一球大-5'],
  },
  {
    title: '混合、色波与梭哈',
    body: '和值大、小、单、双及大单、大双、小单、小双按混合玩法独立计注；极小为0–5，极大为22–27。三球形态可投豹子、对子、顺子，色波可投红波、绿波、蓝波。梭哈须单独提交，按最大整数金额下注并保留零头。0、13、14、27的灰波返本以房间服务端配置为准。',
    examples: ['极小5', '红波5', '余额100.5，大梭哈 → 大/100，余额0.5'],
  },
  {
    title: '三套玩法共同规则',
    body: '8,9,0与9,0,1均按顺子结算；单点数字每期最多选择10个不同点数。三套玩法的13/14、反向下注和有效流水规则不同，必须按明确绑定的玩法版本结算。',
  },
]

const pc28VariantSections: Record<'play1' | 'play2' | 'play3', GameManualSection> = {
  play1: {
    title: '玩法一 · 当前13/14扩展',
    body: '以下是当前系统的版本化扩展，并非所给原版说明中的赔率表条款：禁止反向仅指和值大小及和值单双市场，不包含第1–3球的定位两面。总注严格大于1时，13/14两面按1.5倍、13/14组合按1倍；总注严格大于9999时，13/14两面按1倍（覆盖1.5倍）。开13或14时，本期所有下注不计入有效流水。',
    examples: ['13/14两面：>1 为1.5倍；>9999 为1倍', '13/14组合：>1 为1倍'],
  },
  play2: {
    title: '玩法二 · 当前13/14扩展',
    body: '以下是当前系统的版本化扩展，并非所给原版说明中的赔率表条款：禁止反向仅指和值大小及和值单双市场，不包含第1–3球的定位两面。总注严格大于1时，13/14两面按1.5倍；总注严格大于9999时按1倍（覆盖1.5倍）。组合总注严格大于1且开13或14时，组合玩法庄家通吃。',
    examples: ['13/14两面：>1 为1.5倍；>9999 为1倍', '总注>1且开13/14：组合玩法庄家通吃'],
  },
  play3: {
    title: '玩法三 · 当前13/14扩展',
    body: '所给原版说明未规定和值大小/单双反向限制或13/14动态降赔；当前系统保留以下版本化扩展：第1–3球定位两面不属于玩法一、二的禁止反向范围，总注严格大于1时，13/14两面按1.98倍，13/14组合按3.65倍。',
    examples: ['13/14两面：>1 为1.98倍', '13/14组合：>1 为3.65倍'],
  },
}

const pc28VariantComparison: GameManualSection = {
  title: '玩法一、二、三差异',
  body: '原版明确给出三套各自的32项基础赔率，但没有写下列13/14动态条款。当前系统扩展为：玩法一仅和值大小/单双禁止反向，13/14两面严格>1为1.5倍、严格>9999为1倍，组合严格>1为1倍，开13/14全期流水为0；玩法二的两面阈值相同，组合严格>1且开13/14时庄家通吃；玩法三两面严格>1为1.98倍、组合严格>1为3.65倍。三版本均不把球位定位两面纳入禁止反向范围。',
}

const animalSections: GameManualSection[] = [
  {
    title: '六名动物',
    body: '号码1–6对应饿小宝、盒马、票票、虾仔、支小宝、欢猩。第1至第6名可投号码及大小单双；龙虎仅第1至第3名，分别比较1↔6、2↔5、3↔4。',
    examples: ['1/大/5 → 第一名大-5', '123大/5 → 第一名号码1、2、3及大，各5', '1大5 = 1/大/5'],
  },
  {
    title: '梭哈',
    body: '梭哈须单独提交；按展开投注项以最大整数等额下注，无法整除的积分与小数余额保留。',
    examples: ['余额100，1/123/梭哈 → 每项33，余额1'],
  },
]

function racingManual(game: Game): GameManual {
  const bingoVariant = game.id === 'bingo-racing-a' || game.id === 'bingo-racing-b'
  const ready = gameRulesReady(game)
  const segment = game.id === 'bingo-racing-b' ? '后10个' : '前10个'
  const variantName = game.id === 'bingo-racing-b' ? '宾果赛车(B)' : '宾果赛车(A)'
  const platformExtension = game.id === 'bingo-racing-b'
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: bingoVariant && !ready ? 'partial' : 'implemented',
    statusText: bingoVariant && !ready ? '规则版本待核验 · 暂停受理' : '聊天与网投已接入',
    betModes: { chat: ready, web: ready },
    summary: bingoVariant ? `163 ID185提供原始20球开出顺序，并与ID135的同期20球集合交叉核对；${variantName}取${segment}，在该10球窗口内按数值排名为1–10，同时保留真实开出顺序。聊天与网投共用racing-v2。${platformExtension ? ' 宾果赛车(B)是复用同一规则引擎的平台扩展，不属于原版独立彩种。' : ''}${!ready ? ' 当前目录、赔率与助手尚未共同确认当前版本，暂停受理。' : ''}` : '10名赛车规则。聊天指令支持紧凑写法、位置组与整数梭哈；网投面板支持号码、两面、龙虎和冠亚和。',
    sourceURL: bingoVariant ? 'https://www.www-163kai.cc/' : undefined,
    sections: racingSections,
    auditNotes: bingoVariant ? [
      'ID185是有序母源，ID135只核对同期号、20球集合和开奖时点；任一侧缺期、重号、越界或不一致时，该期不写入可结算结果。',
      `${variantName}只有在目录、赔率与助手响应都确认racing-v2且规则就绪时才开放；缺少有效赔率的选项仍单独关闭。`,
      `${variantName}冠亚和的大、小、单、双及3–19分别读取本彩种后台确认赔率；未配置项不会使用通用sum赔率或跨彩种借价。`,
      ...(platformExtension ? ['宾果赛车(B)复用宾果赛车(A)的racing-v2玩法合同；这是平台扩展，不是原版说明中的独立玩法。'] : []),
    ] : undefined,
  }
}

function digitFiveManual(game: Game): GameManual {
  const v3 = isDigit5V3Game(game.id, game.ruleVersion)
  const ready = gameRulesReady(game)
  const sg = game.id === 'sg-ssc'
  const bingoSegments: Record<string, { range: string; example: string }> = {
    'bingo-ssc-1': { range: '第1–5个', example: '原始第1–5号：12、28、35、47、59 → 2、8、5、7、9' },
    'bingo-ssc-2': { range: '第6–10个', example: '原始第6–10号：64、30、75、18、42 → 4、0、5、8、2' },
    'bingo-ssc-3': { range: '第11–15个', example: '原始第11–15号：71、26、53、40、68 → 1、6、3、0、8' },
    'bingo-ssc-4': { range: '第16–20个', example: '原始第16–20号：15、72、39、50、66 → 5、2、9、0、6' },
  }
  const bingoSegment = bingoSegments[game.id]
  const bingoPlatformExtension = game.id === 'bingo-ssc-2' || game.id === 'bingo-ssc-3' || game.id === 'bingo-ssc-4'
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: ready && v3 ? 'implemented' : 'partial',
    statusText: ready && v3 ? '三段形态与龙虎和已接入' : '规则版本待核验 · 暂停受理',
    betModes: { chat: ready, web: ready },
    summary: `5球数字彩，固定使用 digits5-v3。聊天与网投支持球位号码、大小单双、前三/中三/后三形态，以及第一球对第五球的龙、虎、和。总和、总和尾及第二球对第四球龙虎不在当前规则合同内，不开放投注。${sg ? ' SG时时彩由163目录ID64提供唯一号码母源，115的sgssc产品只读校验；任何缺期或分歧都会暂停导入、投注及未核验期结算，不回退为平台自开。' : ''}${bingoSegment ? ` 本彩种按真实开出顺序取宾果20球的${bingoSegment.range}尾数。` : ''}${bingoPlatformExtension ? ' 宾果时时彩(二至四)是复用digits5-v3的平台扩展，不属于原版独立彩种。' : ''}${!ready ? ' 当前规则版本或就绪状态待核验，暂停受理。' : ''}`,
    sourceURL: game.id.startsWith('bingo-') ? 'https://www.www-163kai.cc/' : undefined,
    sections: [
      ...(sg ? [{
        title: 'SG外部开奖校对',
        body: '163目录ID64（站内名称“168SG时时彩”）是唯一号码母源，提供期号、原序五球、开奖时间、有限历史及下一期边界。115的sgssc产品是只读校验源：它可以因缺期、错号、时间或下一期不一致否决导入，但不能替代或补写ID64缺失的号码。最新期、下一期边界及最近连续24期都必须核对通过；已保存的可信结果仍可结算，按匹配的注单来源快照幂等处理，不回退为平台自开。163目录ID169属于另一套结果系统，禁止混用或接续历史。',
      }] : []),
      ...(bingoSegment ? [{
        title: '宾果开奖转换',
        body: `163 ID185提供保留真实开出顺序的宾果20球，ID135核对同期期号、20球集合和开奖时点。本彩种取${bingoSegment.range}原号的个位数作为第1至第5球；原序不可证明、号码不完整或核对不一致时，当期不会生成可结算开奖。`,
        examples: [bingoSegment.example],
      }] : []),
      ...digitFiveV3Sections,
    ],
    auditNotes: ['投注仅在当前彩种明确绑定 digits5-v3 且规则就绪时开放；目录、赔率与助手响应必须使用同一版本。', '赔率全部读取当前房间后台配置；“和”使用独立玩法赔率，缺失时前端与服务端均拒绝投注。', '原版未说明890、901边界；当前明确沿用循环顺子，不套用PC/加拿大28的排除条款。', ...(sg ? ['163目录与115均为第三方聚合展示来源；一致只能形成当前产品的双边核验，不能证明上游独立。ID169与ID64不是同一开奖结果体系。'] : []), ...(bingoSegment ? ['宾果时时彩(一至四)均不接受按数值排序后的20号数组；必须由服务端完成ID185原始顺序与ID135集合的同期交叉核对。'] : []), ...(bingoPlatformExtension ? ['宾果时时彩(二至四)沿用宾果时时彩(一)的digits5-v3玩法合同，但属于平台扩展，不是原版说明中的独立玩法。'] : [])],
  }
}

function digitThreeManual(game: Game): GameManual {
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: 'implemented',
    statusText: '本地三球规则已接入',
    betModes: { chat: game.rulesReady !== false, web: game.rulesReady !== false },
    summary: '本地三球数字彩规则：号码、两面、总和、总和尾、第一球龙虎及三球形态均使用同一版本化结算合同。',
    sections: digitThreeSections,
    auditNotes: ['这两个官方三位彩不在本次外部手册内；此处展示的是当前本地已启用规则，不把它们套用到PC28。'],
  }
}

function pc28Manual(game: Game): GameManual {
  const version = pc28RuleVersionForGame(game.id)
  const variantKey = version === 'pc28-v1' ? 'play1' : version === 'pc28-v2' ? 'play2' : 'play3'
  const variantLabel = version === 'pc28-v1' ? '玩法一' : version === 'pc28-v2' ? '玩法二' : '玩法三'
  const versionReady = isPC28RuleVersion(game.id, game.ruleVersion)
  const sourceName = game.sourceName || (game.sourceKind === 'external' || game.sourceKind === 'official' ? '外部开奖源' : '王者开奖')
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: 'implemented',
    statusText: `${variantLabel} · 聊天与PC专用网投已接入`,
    betModes: { chat: game.rulesReady !== false && versionReady, web: game.rulesReady !== false && versionReady },
    summary: `每期开出3个0–9数字并计算和值0–27。${game.title}固定绑定${variantLabel}（${version}），开奖页展示三球与和值；当前开奖来源为“${sourceName}”。`,
    sections: [
      { title: '开奖来源与结果', body: `当前开奖来源显示为“${sourceName}”。开奖结果必须恰好包含3个0–9数字，系统同时展示三球、和值、和值大小单双以及第一球对第三球的龙虎和。` },
      ...pc28Sections,
      pc28VariantSections[variantKey],
      pc28VariantComparison,
    ],
    auditNotes: [`彩种ID与版本固定映射：pc-canada→pc28-v1、canada-28→pc28-v2、canada-20→pc28-v3；服务端返回版本不匹配时暂停投注。`, '网投选项逐项读取当前房间服务端赔率；任一原子玩法缺失赔率时单独禁用，不使用默认赔率。', '原版明确890与901算顺子；PC28专用结算按循环顺子处理，019作为同一组号码同样命中。'],
  }
}

function markSixManual(game: Game): GameManual {
  const ready = gameRulesReady(game)
  const product = markSixProducts[game.id as keyof typeof markSixProducts]
  const fiveElements = '以特码所在原版固定号码组判断：金=06、07、20、21、28、29、36、37；木=02、03、10、11、18、19、32、33、40、41、48、49；水=08、09、16、17、24、25、38、39、46、47；火=04、05、12、13、26、27、34、35、42、43；土=01、14、15、22、23、30、31、44、45。开奖来源不会改变规则版本中的五行号码表。'
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: ready ? 'implemented' : 'partial',
    statusText: ready ? '完整详细网投已接入' : '完整规则已实现 · 当前房间待核验',
    betModes: { chat: false, web: ready },
    summary: `${product.summary}；前6个为正码，第7个为特码。${product.version}已覆盖${game.id === 'bingo-mark-six' ? '原版全部' : '完整'}详细网投市场，本彩种不解析聊天指令。`,
    sourceURL: 'https://www.www-163kai.cc/',
    sections: [
      {
        title: '开奖来源与取号',
        body: product.source,
        examples: [product.example],
      },
      {
        title: '特码与两面',
        body: '第7个号码为特码。特码大小：01–24小、25–48大；特码单双按号码判断；合数大小按十位与个位之和1–6为合小、7–12为合大；合数单双按位数和判断；尾数大小为0–4尾小、5–9尾大。以上玩法开49均为和局返本。',
        examples: ['特码34 → 大、双、合大、合单、尾小', '特码49 → 上述两面全部和局返本'],
      },
      {
        title: '特码分组与半特',
        body: '天肖为牛、兔、龙、马、猴、猪，地肖为鼠、虎、蛇、羊、鸡、狗；前肖为鼠、牛、虎、兔、龙、蛇，后肖为马、羊、猴、鸡、狗、猪；家肖为牛、马、羊、鸡、狗、猪，野肖为鼠、虎、兔、龙、蛇、猴。这三类分组开49为和局返本。半特把大小与单双组合成大单、大双、小单、小双；半特开49则不中奖。',
      },
      {
        title: '总和',
        body: '7个开奖号码相加：总和大为175及以上，总和小为174及以下；总和单双按总分奇偶判断。总和玩法使用全部7个号码，不把特码单独剔除。',
      },
      {
        title: '色波、半波与半半波',
        body: '特码按固定红、蓝、绿波表判断。半波把颜色与大小或单双组合；半半波再把颜色、大小、单双三项组合。特码49属于绿波，但在半波及半半波中按和局返本处理。',
        examples: ['红波：01、02、07、08、12、13、18、19、23、24、29、30、34、35、40、45、46', '蓝波：03、04、09、10、14、15、20、25、26、31、36、37、41、42、47、48', '绿波：05、06、11、16、17、21、22、27、28、32、33、38、39、43、44、49'],
      },
      {
        title: '生肖与合肖',
        body: '生肖按开奖日期所在的农历年动态轮换：当年生肖对应01、13、25、37、49，之后每个号码按鼠、牛、虎、兔、龙、蛇、马、羊、猴、鸡、狗、猪的循环逆序分配。特码生肖命中所选生肖即中奖；合肖选择2–11个生肖，特码49统一和局返本。历史开奖按当期开奖日计算，不能用今天的生肖表重算。',
        examples: ['2026马年：马=01、13、25、37、49；猴=11、23、35、47；猪=08、20、32、44'],
      },
      {
        title: '特码头数与尾数',
        body: '头数按十位分为0头至4头，其中0头为01–09、4头为40–49；尾数按个位分为0尾至9尾。特码落在所选头数或尾数即中奖。',
        examples: ['特码21 → 2头、1尾'],
      },
      {
        title: '正码、正码特与正码1–6',
        body: '前6个号码为正码。正码选号只要出现在任一正码位置即中奖；正码特按指定的正1至正6位置和号码精确命中。正码1–6还可按每个指定位置投注大小、单双、合数大小、合数单双、尾数大小及色波；该位置开49时，两面属性按和局返本。',
      },
      {
        title: '五行',
        body: fiveElements,
      },
      {
        title: '一肖、一尾与总肖',
        body: '一肖和一尾使用全部7个开奖号码，只要出现一次或多次都只派一次；一肖中的49照常算生肖，不作和局。一尾0–9分别使用独立赔率。总肖统计本期7个号码覆盖的不同生肖数量，可投2–7肖及总肖单双，49照常参与统计。',
      },
      {
        title: '正肖与七色波',
        body: '正肖只统计前6个正码，命中同一生肖的正码有几个，盈利部分就按命中次数倍增，49照常算生肖。七色波统计7个号码的颜色：每个正码计1分、特码计1.5分，最高颜色中奖；三种“正码3比3、特码为第三色”的情形是和局，可单独投注和局。',
      },
      {
        title: '自选不中',
        body: '选择5–11个不同号码组成一注；只要当期7个开奖号码全部不在所选组合内即中奖，任一所选号码开出即不中奖。5–11不中按选择数量使用各自明确配置的赔率与限额。',
      },
      {
        title: '连肖与连尾',
        body: '连肖选择2–5个生肖，每个所选生肖都必须至少在本期7个号码中出现；连尾选择2–5个尾数，每个所选尾数都必须至少出现。一个生肖或尾数出现多次仍只算一次，49照常参与。会员提交一张公开父玩法组合票，服务端从所选组件的后台内部定价行中取最低有效赔率；内部定价行不能单独下注。',
      },
      {
        title: '连码',
        body: '四全中、三全中、二全中均只用前6个正码判断；三中二有“中二”与“中三”两档赔率；二中特有“中二正码”与“一正一码特码”两档赔率；特串必须一号命中特码、另一号命中任一正码。三中二与二中特均是一张票、只扣款一次，两档赔率会同时冻结并按互斥结果选用；组合必须选足规定数量且号码不得重复。',
      },
      {
        title: '赔率、内部定价与限额',
        body: '普通公开玩法使用自身定价行；组合父玩法使用本注所需的全部后台内部定价行。每个必要行都必须具有有效赔率、单注最低、单注最高、会员单期与全房单期限额。连肖、连尾、三中二和二中特的内部定价行只供管理端报价与限额配置，会员不能直接提交；缺少任一必要配置时整张公开玩法保持关闭，不采用默认赔率。',
      },
    ],
    auditNotes: [
      `${product.version}已覆盖一尾、2–11合肖、正肖、2–5连肖/连尾、三中二、二中特及5–11不中，连同核心市场组成完整详细网投合同。`,
      '正肖按前6个正码命中数倍增净赢部分，本金只返还一次；三中二、二中特的一张票会冻结两档赔率且只扣款一次。',
      '连肖、连尾及双档连码的组件价格使用后台内部定价行；这些行只用于报价和限额校验，不能成为会员独立注单。',
      '赔率与四类限额以当前房间后台显式配置为准；缺价、无效价、缺少必要组件或限额不完整都会关闭对应公开玩法，不使用默认值。',
    ],
  }
}

export function manualForGame(game: Game): GameManual {
  if (racingIDs.has(game.id)) return racingManual(game)
  if (digitFiveIDs.has(game.id)) return digitFiveManual(game)
  if (digitThreeIDs.has(game.id)) return digitThreeManual(game)
  if (pc28IDs.has(game.id)) return pc28Manual(game)
  if (markSixIDs.has(game.id)) return markSixManual(game)
  return {
    id: game.id,
    gameId: game.id,
    title: game.title,
    status: 'reference',
    statusText: '完整玩法待配置 · 暂停受理',
    betModes: { chat: false, web: false },
    summary: '该彩种尚未形成解析、赔率、限额与结算一致的规则版本。当前仅展示彩种，不接受投注。',
    sections: [{ title: '安全说明', body: '不能根据名称、开奖号码长度或其他彩种规则推测投注含义；待管理员完成规则与赔率配置后再开放。' }],
  }
}

export const referenceManuals: GameManual[] = [
  {
    id: 'reference-pc-dandan', title: 'PC蛋蛋', status: 'reference', statusText: '参考手册 · 本地尚无独立彩种ID', betModes: { chat: false, web: false },
    summary: '0–27、包三、三球定位、混合、色波与梭哈规则；需绑定明确开奖源及玩法版本后再开放。', sections: pc28Sections,
  },
  {
    id: 'reference-pc28-play-1', title: '加拿大28 · 玩法一', status: 'reference', statusText: '版本对照 · 已绑定 pc-canada', betModes: { chat: false, web: false },
    summary: 'pc-canada 的 pc28-v1 对照规则：含禁止两面反向下注、13/14特殊赔率与有效流水条款。', sections: [...pc28Sections, pc28VariantSections.play1],
    auditNotes: ['开13/14时，本期所有下注不计有效流水；总注阈值与赔率须保存到注单规则快照。'],
  },
  {
    id: 'reference-pc28-play-2', title: '加拿大28 · 玩法二', status: 'reference', statusText: '版本对照 · 已绑定 canada-28', betModes: { chat: false, web: false },
    summary: 'canada-28 的 pc28-v2 对照规则：含禁止两面反向下注及13/14组合庄家通吃条款。', sections: [...pc28Sections, pc28VariantSections.play2],
    auditNotes: ['总注严格大于1且开13/14时，组合庄家通吃；不得与玩法一或三共用未版本化结算。'],
  },
  {
    id: 'reference-pc28-play-3', title: '加拿大28 · 玩法三', status: 'reference', statusText: '版本对照 · 已绑定 canada-20', betModes: { chat: false, web: false },
    summary: 'canada-20 的 pc28-v3 对照规则：13/14两面与组合使用独立特殊赔率。', sections: [...pc28Sections, pc28VariantSections.play3],
  },
  {
    id: 'reference-animal-1m', title: '动物运动会', status: 'reference', statusText: '本地尚无彩种与开奖源', betModes: { chat: false, web: false },
    summary: '目标为6名动物、每1分钟开奖。大/小分界、赔率、期号与官方接口仍待确认。', sourceURL: 'https://www.dongwuhui.com/', sections: animalSections,
  },
  {
    id: 'reference-animal-5m', title: '五分运动会', status: 'reference', statusText: '本地尚无彩种与开奖源', betModes: { chat: false, web: false },
    summary: '目标为6名动物、约每5分40秒开奖。大/小分界、赔率、期号与官方接口仍待确认。', sourceURL: 'https://www.dongwuhui.com/', sections: animalSections,
  },
]

export function gameManualOptions(games: Game[]) {
  const live = games.map(manualForGame)
  const liveIDs = new Set(games.map(game => game.id))
  const documentedGames = [
    ['speed-racing', '极速赛车'], ['au-lucky-10', '澳洲幸运10'], ['au-lucky-5', '澳洲幸运5'],
    ['speed-ssc', '极速时时彩'], ['bingo-racing-a', '宾果赛车(A)'], ['bingo-racing-b', '宾果赛车(B)'],
    ['bingo-ssc-1', '宾果时时彩(一)'], ['bingo-ssc-2', '宾果时时彩(二)'], ['bingo-ssc-3', '宾果时时彩(三)'], ['bingo-ssc-4', '宾果时时彩(四)'],
    ['speed-fly', '极速飞艇'], ['pc-canada', 'PC加拿大'], ['canada-28', '加拿大28'], ['canada-20', '加拿大2.0'],
  ] as const
  const inactive = documentedGames.filter(([id]) => !liveIDs.has(id)).map(([id, title]) => {
    const manual = manualForGame({ id, title } as Game)
    return {
      ...manual,
      id: `manual-${id}`,
      gameId: undefined,
      betModes: { chat: false, web: false },
      status: 'reference' as const,
      statusText: `${manual.statusText} · 当前房间未启用`,
    }
  })
  return [...live, ...inactive, ...referenceManuals]
}
