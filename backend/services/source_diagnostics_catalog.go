package services

import (
	"backend/data/models/lottery"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const source163Base = "http://23.97.72.253:50163"
const source163LatestPath = "/api/homePage/gameNewDataForLotteryHall"
const source163HistoryPath = "/api/complex/selDataByGameIdAndCount"
const source163EvidenceAt = "2026-09-04T00:01:32+08:00"

func sourceDiagnosticSpecs() []sourceDiagnosticSpec {
	result := make([]sourceDiagnosticSpec, 0, 112)
	for _, binding := range append(append([]api168Binding{}, api168HighFreqBindings...), api168MarkSixBindings...) {
		b := binding
		path, _ := api168Paths(b.Series)
		groups, candidate, relation := []string{"现用外部来源"}, false, sourceRelationProduction
		if _, migrated := source163MirrorBindingForGame(b.GameID); migrated {
			groups, candidate, relation = []string{"历史核对来源"}, true, sourceRelationHistorical
		}
		spec := sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "168:" + b.LotCode, Name: "168 · " + diagnosticGameName(b.GameID), Provider: "168", Groups: groups, Candidate: candidate, Relation: relation, GameIDs: []string{b.GameID}, Endpoint: api168Base + path}, binding: &b, count: 10, min: 1, max: 10, unique: true, staleAfter: 30 * time.Minute}
		if b.Series == api168SSC {
			spec.count, spec.min, spec.max, spec.unique = 5, 0, 9, false
		}
		if b.Series == api168LHC {
			spec.count, spec.max, spec.staleAfter = 7, 49, 8*24*time.Hour
		}
		if b.GameID == "fly-racing" {
			spec.staleAfter = source163FlyRacingMaxAge
		}
		result = append(result, spec)
	}
	bingo := api168Binding{LotCode: "10047", Series: api168KL8}
	path, _ := api168Paths(api168KL8)
	result = append(result,
		sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "168:10047", Name: "168 台湾宾果（旧集合）", Provider: "168", Groups: []string{"历史核对来源"}, Candidate: true, Relation: sourceRelationHistorical, GameIDs: []string{"bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "bingo-racing-a", "bingo-racing-b", "bingo-mark-six"}, Endpoint: api168Base + path, Warning: "旧来源仅保留核对；集合不能证明原始出球顺序，禁止作为当前派生彩写入源", WarningPersistent: true}, count: 20, min: 1, max: 80, unique: true, staleAfter: 9 * time.Hour, binding: &bingo},
		sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "sg-ssc-verified", Name: sgSSCVerifiedSourceName, Provider: "163＋115", Groups: []string{"163生产适配", "现用双源来源"}, Relation: sourceRelationProduction, GameIDs: []string{"sg-ssc"}, Endpoint: source163Base + source163LatestPath}, count: 5, min: 0, max: 9, staleAfter: 15 * time.Minute},
		sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "bingo-ordered-163", Name: bingo163OrderedSourceName, Provider: "163宾果双源", Groups: []string{"163生产适配", "现用双源来源"}, Relation: sourceRelationProduction, GameIDs: []string{"bingo-ssc-1", "bingo-ssc-2", "bingo-ssc-3", "bingo-ssc-4", "bingo-racing-a", "bingo-racing-b", "bingo-mark-six"}, Endpoint: source163Base + source163LatestPath, Warning: "最终生产口径为163 ID185原始球序与ID135同期20球集合交叉；任一侧异常时停止写入"}, count: 20, min: 1, max: 80, unique: true, staleAfter: 9 * time.Hour},
		sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "bingo-ordered", Name: bingoVerifiedSourceName + "（旧）", Provider: "168＋jyb.one", Groups: []string{"历史核对来源"}, Candidate: true, Relation: sourceRelationHistorical, GameIDs: []string{"bingo-ssc-1", "bingo-racing-a", "bingo-mark-six"}, Endpoint: bingoOrderedHistoryURL, Warning: "旧双源只保留历史核对，不再作为生产写入源", WarningPersistent: true}, count: 20, min: 1, max: 80, unique: true, staleAfter: 9 * time.Hour},
	)
	for _, game := range officialGames {
		spec := sourceDiagnosticSpec{SourceDiagnosticSource: SourceDiagnosticSource{Key: "official:" + game.ID, Name: game.SourceName + " · " + game.Name, Provider: game.SourceName, Groups: []string{"现用官方目录来源"}, Relation: sourceRelationProduction, GameIDs: []string{game.ID}}, count: 3, min: 0, max: 9, staleAfter: 48 * time.Hour}
		switch game.ID {
		case "official-fc3d", "official-kl8":
			spec.Endpoint = "https://www.cwl.gov.cn/cwl_admin/front/cwlkj/search/kjxx/findDrawNotice"
			if game.ID == "official-kl8" {
				spec.count, spec.min, spec.max, spec.unique = 20, 1, 80, true
			}
		case "official-pl3", "official-qxc":
			spec.Endpoint = "https://webapi.sporttery.cn/gateway/lottery/getHistoryPageListV1.qry"
			if game.ID == "official-qxc" {
				spec.count, spec.max, spec.staleAfter = 7, 14, 8*24*time.Hour
			}
		case "official-tw-bingo":
			spec.Endpoint = "https://api.taiwanlottery.com/TLCAPIWeB/Lottery/BingoResult"
			spec.count, spec.min, spec.max, spec.unique, spec.staleAfter = 20, 1, 80, true, 9*time.Hour
			spec.Warning = "原始接口样本缺少逐期开奖时间；诊断不会以当前时间伪造新鲜度"
			spec.WarningPersistent = true
		default:
			spec.Endpoint = "https://api.taiwanlottery.com/TLCAPIWeB/Lottery/LatestResult"
			spec.count, spec.min, spec.max, spec.unique, spec.staleAfter = 7, 1, 49, true, 8*24*time.Hour
			if game.ID == "official-tw-super-lotto" {
				spec.max, spec.unique = 38, false
			}
			if game.ID == "official-tw-daily539" {
				spec.count, spec.max, spec.staleAfter = 5, 39, 48*time.Hour
			}
		}
		result = append(result, spec)
	}
	// Snapshot of the 88-item public directory observed on 2026-09-04, not a
	// new game seed or source binding.
	// A repeated upstream ID (69) is represented once with both group labels.
	for _, row := range []struct {
		id                                  int
		name, group, family, games, warning string
	}{
		{56, "168极速赛车", "极速彩", "racing", "speed-racing", ""}, {55, "168极速时时彩", "极速彩", "ssc", "speed-ssc", "历史接口曾忽略count=3并返回500条；本地只取有限样本"},
		{57, "加拿大28", "PC", "digits3", "pc-canada,canada-28,canada-20", ""},
		{4, "天津时时彩", "全国彩", "ssc", "", ""}, {5, "重庆欢乐生肖", "全国彩", "ssc", "", ""}, {7, "新疆时时彩", "全国彩", "ssc", "", ""},
		{9, "PK拾", "全国彩", "racing", "", ""}, {10, "江苏快三", "全国彩", "dice", "", ""}, {12, "吉林快3", "全国彩", "dice", "", ""},
		{15, "广东11选5", "全国彩", "eleven", "", ""}, {17, "广西快三", "全国彩", "dice", "", ""}, {23, "北京快乐8", "全国彩", "keno", "", ""},
		{25, "广西快乐十分", "全国彩", "eight", "", ""}, {27, "广东快乐十分", "全国彩", "eight", "", ""}, {28, "幸运农场", "全国彩", "eight", "", ""},
		{42, "PC蛋蛋", "PC", "digits3", "", "固定目录有三球数据，但尚未建立与本地三款加拿大28玩法的同款身份关系"}, {44, "腾讯分分彩", "全国彩", "ssc", "", ""},
		{50, "PK10牛牛", "高频彩", "racing", "", ""}, {51, "文莱幸运5", "境外彩", "ssc", "", ""}, {52, "文莱幸运8", "境外彩", "eight", "", ""},
		{53, "文莱幸运10", "境外彩", "racing", "", ""}, {54, "文莱幸运20", "境外彩", "keno", "", ""}, {66, "极速快乐十分", "极速彩", "eight", "", ""},
		{142, "哈希牛牛", "哈希彩", "racing", "", ""}, {151, "哈希牛牛2", "哈希彩", "racing", "", ""},
		{61, "168极速飞艇", "极速彩", "racing", "speed-fly", ""}, {62, "168极速牛牛", "极速彩", "racing", "", ""}, {69, "168极速六合彩", "极速彩|六合彩", "marksix", "", ""},
		{68, "168极速11选5", "极速彩", "eleven", "", ""}, {67, "168极速快乐8", "极速彩", "keno", "", ""}, {63, "168极速快3", "极速彩", "dice", "", ""},
		{37, "超速赛车", "极速彩", "racing", "", "2026-09-04核查时最新停在2024-08-09"}, {36, "超速时时彩", "极速彩", "ssc", "", "2026-09-04核查时最新停在2024-08-09"},
		{41, "超速飞艇", "极速彩", "racing", "", "2026-09-04核查时最新停在2024-08-09"}, {48, "超速牛牛", "极速彩", "racing", "", "2026-09-04核查时最新停在2024-08-09"},
		{132, "哈希赛车", "哈希彩", "racing", "", ""}, {130, "哈希时时彩", "哈希彩", "ssc", "", ""}, {133, "哈希幸运5", "哈希彩", "ssc", "", ""}, {134, "哈希幸运10", "哈希彩", "racing", "", ""}, {139, "哈希快3", "哈希彩", "dice", "", ""},
		{180, "动物运动会", "163官方彩", "sports", "", "号码范围仅做宽松结构检查，尚无本地玩法契约"}, {181, "三分运动会", "163官方彩", "sports", "", "2026-09-04目录中ID181与ID182同名；号码仅作宽松结构检查，身份与本地玩法均未验收"}, {182, "三分运动会", "163官方彩", "sports", "", "2026-09-04目录中ID181与ID182同名；号码仅作宽松结构检查，身份与本地玩法均未验收"},
		{160, "极速赛车", "163官方彩", "racing", "speed-racing", ""}, {161, "极速牛牛", "163官方彩", "racing", "", ""}, {162, "极速飞艇", "163官方彩", "racing", "speed-fly", ""},
		{163, "澳洲幸运10", "163官方彩", "racing", "au-lucky-10", ""}, {164, "幸运飞艇", "163官方彩", "racing", "fly-racing", ""}, {165, "SG飞艇", "163官方彩", "racing", "sg-fly", ""},
		{166, "幸运时时彩", "163官方彩", "ssc", "", "幸运时时彩不等于本地极速时时彩或SG时时彩"}, {167, "极速时时彩", "163官方彩", "ssc", "speed-ssc", ""}, {168, "澳洲幸运5", "163官方彩", "ssc", "au-lucky-5", ""},
		{169, "SG时时彩", "163官方彩", "ssc", "sg-ssc", "目标产品，与现源163:64同期25期均不同；尚未切换，不能混用旧历史"}, {170, "SG快3", "163官方彩", "dice", "", ""}, {171, "极速快3", "163官方彩", "dice", "", ""},
		{172, "极速快乐8", "163官方彩", "keno", "", ""}, {173, "澳洲幸运20", "163官方彩", "keno", "", ""}, {174, "极速六合彩", "163官方彩", "marksix", "", ""}, {175, "极速11选5", "163官方彩", "eleven", "", ""}, {176, "澳洲幸运8", "163官方彩", "eight", "", ""},
		{38, "168幸运飞艇", "高频彩", "racing", "fly-racing", ""}, {59, "168幸运时时彩", "高频彩", "ssc", "", "幸运时时彩不等于本地极速时时彩或SG时时彩"}, {58, "168SG飞艇", "高频彩", "racing", "sg-fly", ""},
		{64, "168SG时时彩", "高频彩", "ssc", "sg-ssc", "2026-09-03现用SG最近5期与本项一致；与163:169不同，不证明上游独立"}, {65, "168SG快3", "高频彩", "dice", "", ""},
		{31, "168澳洲幸运5", "境外彩", "ssc", "au-lucky-5", ""}, {32, "168澳洲幸运8", "境外彩", "eight", "", ""}, {33, "168澳洲幸运10", "境外彩", "racing", "au-lucky-10", ""}, {34, "168澳洲幸运20", "境外彩", "keno", "", ""},
		{135, "台湾宾果", "境外彩", "keno", "bingo-ssc-1,bingo-ssc-2,bingo-ssc-3,bingo-ssc-4,bingo-racing-a,bingo-racing-b,bingo-mark-six,official-tw-bingo", "7个宾果派生彩的163升序20球集合核对源；生产顺序必须通过163 ID185同期号、集合与开奖时点交叉；对台湾彩券原始宾果仅作交叉核查"}, {136, "台湾威力彩", "境外彩", "super", "official-tw-super-lotto", ""}, {137, "台湾今彩539", "境外彩", "five39", "official-tw-daily539", "2026-09-04核查时最新停在2026-07-25"},
		{185, "台湾宾果(开奖顺序)", "境外彩", "keno", "bingo-ssc-1,bingo-ssc-2,bingo-ssc-3,bingo-ssc-4,bingo-racing-a,bingo-racing-b,bingo-mark-six", ""},
		{186, "宾果六合彩", "境外彩", "marksix", "bingo-mark-six", "上游派生对照：ID185原序中前7个01–49；仅用于核查本地转换，不作独立母源"},
		{187, "宾果赛车(A)", "境外彩", "racing", "bingo-racing-a", "上游派生对照：ID185前10球在该窗口内按大小取名次；仅用于核查本地转换"},
		{188, "宾果赛车(B)", "境外彩", "racing", "bingo-racing-b", "上游派生对照：ID185后10球在该窗口内按大小取名次；仅用于核查本地转换"},
		{189, "宾果时时彩(一)", "境外彩", "ssc", "bingo-ssc-1", "上游派生对照：ID185第1–5球取尾数；仅用于核查本地转换"},
		{190, "宾果时时彩(二)", "境外彩", "ssc", "bingo-ssc-2", "上游派生对照：ID185第6–10球取尾数；仅用于核查本地转换"},
		{191, "宾果时时彩(三)", "境外彩", "ssc", "bingo-ssc-3", "上游派生对照：ID185第11–15球取尾数；仅用于核查本地转换"},
		{192, "宾果时时彩(四)", "境外彩", "ssc", "bingo-ssc-4", "上游派生对照：ID185第16–20球取尾数；仅用于核查本地转换"},
		{1, "福彩3D", "全国彩", "digits3", "official-fc3d", ""}, {2, "排列3", "全国彩", "digits3", "official-pl3", ""}, {20, "七星彩", "全国彩", "qxc", "official-qxc", ""},
		{18, "香港六合彩", "六合彩", "marksix", "hong-kong-mark-six", ""}, {141, "福彩快乐8六合彩", "六合彩", "marksix", "happy8-mark-six", "163派生7球私盘，不是福彩快乐8原始20球；当前按ID141直接七球合同使用，不做20球推导"},
		{140, "新澳门六合彩", "六合彩", "marksix", "new-macau-mark-six", ""}, {70, "澳门六合彩", "六合彩", "marksix", "old-macau-mark-six", ""},
		{60, "台湾大乐透", "六合彩", "marksix", "official-tw-lotto649", "2026-09-04核查时当前为空、历史停在2025-01-31"},
	} {
		groups := strings.Split(row.group, "|")
		relation := source163DiagnosticRelation(row.id)
		candidate := relation != sourceRelationProduction
		if source163MirrorProductionID(row.id) {
			groups = append([]string{"163生产适配"}, groups...)
		}
		warning := row.warning
		if warning == "" {
			warning = source163DiagnosticWarning(row.id)
		}
		source := SourceDiagnosticSource{Key: "163:" + strconv.Itoa(row.id), Name: row.name, Provider: "163", Groups: groups, Candidate: candidate, Relation: relation, GameIDs: []string{}, Endpoint: source163Base + source163LatestPath, UpstreamGameID: row.id, Warning: warning, WarningPersistent: source163DiagnosticWarningPersistent(row.id)}
		if row.id == bingo163UpstreamGameID {
			source.GameRelations = map[string]string{"official-tw-bingo": sourceRelationCrossCheck}
		}
		spec := sourceDiagnosticSpec{SourceDiagnosticSource: source, count: 10, min: 1, max: 10, unique: true, staleAfter: 30 * time.Minute}
		if row.games != "" {
			spec.GameIDs = strings.Split(row.games, ",")
		}
		if warning != "" {
			spec.WarningCheckedAt = source163EvidenceAt
		}
		switch row.family {
		case "ssc":
			spec.count, spec.min, spec.max, spec.unique = 5, 0, 9, false
		case "digits3":
			spec.count, spec.min, spec.max, spec.unique, spec.staleAfter = 3, 0, 9, false, 48*time.Hour
		case "dice":
			spec.count, spec.max, spec.unique = 3, 6, false
		case "marksix":
			spec.count, spec.max = 7, 49
		case "eleven":
			spec.count, spec.max = 5, 11
		case "keno":
			spec.count, spec.max = 20, 80
		case "eight":
			spec.count, spec.max = 8, 20
		case "sports":
			spec.count, spec.min, spec.max, spec.unique = 6, 0, 80, false
		case "super":
			spec.count, spec.max, spec.unique, spec.staleAfter = 7, 38, false, 8*24*time.Hour
		case "five39":
			spec.count, spec.max, spec.staleAfter = 5, 39, 48*time.Hour
		case "qxc":
			spec.count, spec.min, spec.max, spec.unique, spec.staleAfter = 7, 0, 14, false, 8*24*time.Hour
		}
		switch row.id {
		case 57, 64:
			spec.staleAfter = source163MirrorMaxAge
		case 18, 60:
			spec.staleAfter = 8 * 24 * time.Hour
		case 70, 140, 141:
			spec.staleAfter = 48 * time.Hour
		case 135, 185, 186, 187, 188, 189, 190, 191, 192:
			spec.staleAfter = 9 * time.Hour
		case 38, 164:
			spec.staleAfter = source163FlyRacingMaxAge
		}
		result = append(result, spec)
	}
	return result
}

func diagnosticGameName(id string) string {
	for _, game := range defaultGames {
		if game.ID == id {
			return game.Name
		}
	}
	return id
}

func configuredSourceDiagnosticKey(game lottery.Game) string {
	if game.SourceKind == "platform" || game.SourceKind == "simulated" {
		return ""
	}
	if binding, ok := source163MirrorBindingForGame(game.ID); ok && source163MirrorBound(&game, binding) {
		return "163:" + strconv.Itoa(binding.UpstreamGameID)
	}
	if binding, ok := source163MarkSixBindingForGame(game.ID); ok && source163MarkSixBound(&game, binding) {
		return "163:" + strconv.Itoa(binding.UpstreamGameID)
	}
	if binding, ok := source163PC28BindingForGame(game.ID); ok && source163PC28Bound(&game, binding) {
		return "163:" + strconv.Itoa(binding.UpstreamGameID)
	}
	if binding, ok := bingo163BindingForGame(game.ID); ok && bingo163SourceBound(&game, binding) {
		if binding.RequiresOrderedSource {
			return "bingo-ordered-163"
		}
		return "163:" + strconv.Itoa(bingo163UpstreamGameID)
	}
	if game.ID == "sg-ssc" && sgSSCSourceBound(&game) {
		return "sg-ssc-verified"
	}
	if binding, ok := api168BingoBindingForGame(game.ID); ok {
		name, endpoint, _, _ := bingoBindingSourceDefaults(binding)
		if game.SourceKind == "external" && game.SourceName == name && game.SourceURL == endpoint {
			if binding.RequiresOrderedSource {
				return "bingo-ordered"
			}
			return "168:10047"
		}
	}
	for _, spec := range sourceDiagnosticSpecs() {
		if spec.binding != nil && spec.binding.GameID == game.ID && game.SourceKind == "external" && game.SourceName == "168开奖网" && game.SourceURL == "https://kj138138.com/view/api/index.html" {
			return spec.Key
		}
	}
	for _, expected := range officialGames {
		if expected.ID == game.ID && expected.SourceKind == game.SourceKind && expected.SourceName == game.SourceName && expected.SourceURL == game.SourceURL {
			return "official:" + game.ID
		}
	}
	return ""
}

func source163MirrorProductionID(upstreamGameID int) bool {
	if upstreamGameID == bingo163SetUpstreamGameID || upstreamGameID == bingo163OrderedUpstreamGameID || upstreamGameID == sgSSC163MotherBinding.UpstreamGameID {
		return true
	}
	if _, ok := source163MarkSixBindingForUpstream(upstreamGameID); ok {
		return true
	}
	for _, binding := range source163MirrorBindings {
		if binding.UpstreamGameID == upstreamGameID {
			return true
		}
	}
	for _, binding := range source163PC28Bindings {
		if binding.UpstreamGameID == upstreamGameID {
			return true
		}
	}
	return false
}

func source163DiagnosticRelation(upstreamGameID int) string {
	if source163MirrorProductionID(upstreamGameID) {
		return sourceRelationProduction
	}
	switch upstreamGameID {
	case 160, 162, 163, 164, 165, 167, 168, 169:
		return sourceRelationDifferentProduct
	case 1, 2, 20, 136, 186, 187, 188, 189, 190, 191, 192:
		return sourceRelationCrossCheck
	case 37, 36, 41, 48, 60, 137:
		return sourceRelationUnavailable
	default:
		return sourceRelationCatalogOnly
	}
}

func source163DiagnosticWarning(upstreamGameID int) string {
	switch upstreamGameID {
	case 160, 162, 163, 164, 165, 167, 168:
		return "2026-09-04与163站内同名168项核对25个共同期号，号码25/25不同；属于不同开奖结果体系，禁止作为同款备用源"
	case 1, 2, 20, 136:
		return "163为聚合接口，仅可辅助交叉核查；当前发行方来源应保留，尚未验收为替代主源"
	default:
		return ""
	}
}

func source163DiagnosticWarningPersistent(upstreamGameID int) bool {
	if upstreamGameID == 181 || upstreamGameID == 182 {
		return true
	}
	switch source163DiagnosticRelation(upstreamGameID) {
	case sourceRelationDifferentProduct, sourceRelationCrossCheck, sourceRelationUnverified:
		return true
	default:
		return false
	}
}

// source163DirectoryAssessment is intentionally scoped to the fixed directory
// audited on 2026-09-04. "not_found" never claims that no provider
// exists elsewhere, and current binding is derived from the database metadata.
func source163DirectoryAssessment(game lottery.Game, configuredKey string) (status, message string) {
	if strings.HasPrefix(configuredKey, "163:") || configuredKey == "bingo-ordered-163" || configuredKey == "sg-ssc-verified" {
		return source163StatusCurrent, "当前数据库已绑定163母源；以“当前使用”标记和运行同步状态为准"
	}
	if _, ok := source163MirrorBindingForGame(game.ID); ok {
		return source163StatusVerifiedCandidate, "163同款生产适配已登记，但当前数据库未绑定该来源"
	}
	if binding, ok := source163MarkSixBindingForGame(game.ID); ok {
		return source163StatusVerifiedCandidate, fmt.Sprintf("163 ID%d七球生产适配及独立来源版本已登记，当前数据库未绑定时不会发起写入", binding.UpstreamGameID)
	}
	if _, ok := source163PC28BindingForGame(game.ID); ok {
		return source163StatusVerifiedCandidate, "2026-09-04实时核验：163 ID57为3球0–9、210秒连续母源；三款加拿大28玩法共用开奖、按各自规则版本独立结算，生产适配已登记但当前数据库未绑定"
	}
	if _, ok := bingo163BindingForGame(game.ID); ok {
		return source163StatusVerifiedCandidate, "163台湾宾果生产适配已登记，但当前数据库未绑定该来源"
	}
	switch game.ID {
	case "sg-ssc":
		return source163StatusVerifiedCandidate, "163 ID64与现用产品样本一致，可作同款候选；ID169是不同开奖结果体系"
	case "official-kl8":
		return source163StatusNotFound, "本次163固定目录未找到福彩快乐8原始20球同款；ID141是7球派生品，继续保留中国福彩网来源"
	case "official-tw-daily539":
		return source163StatusUnavailable, "163已登记ID137，但2026-09-04核查时数据停在2026-07-25；继续保留台湾彩券来源"
	case "official-tw-lotto649":
		return source163StatusUnavailable, "163已登记ID60，但2026-09-04核查时当前为空、历史停在2025-01-31；继续保留台湾彩券来源"
	case "official-fc3d", "official-pl3", "official-qxc", "official-tw-super-lotto", "official-tw-bingo":
		return source163StatusCandidateUnverified, "163目录有可交叉核查的聚合项；当前发行方来源继续保留，未验收为替代主源"
	default:
		return source163StatusNotAssessed, "本地彩种尚未纳入本次163固定目录的同款来源评估"
	}
}
