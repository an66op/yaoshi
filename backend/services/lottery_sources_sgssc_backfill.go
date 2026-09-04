package services

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	sgSSCBackfillMaxIssues = 48
	sgSSCBackfillMaxDates  = 2
	sgSSCBackfillMaxAge    = 30 * 24 * time.Hour
)

// A validly parsed but absent/disagreeing target is isolated from other target
// periods. Transport, source identity and malformed-response errors fail the
// whole attempt instead; callers must not import any rows on a non-nil error.
type SGSSCHistoryFailure struct {
	Issue     string
	Error     string
	Permanent bool
}

type SGSSCHistoryVerification struct {
	Draws    []sourceDraw
	Failures []SGSSCHistoryFailure
}

type sgSSCHistoryTargets struct {
	issues []string
	dates  []string
	at     map[string]time.Time
}

func fetchSGSSCVerifiedHistory(ctx context.Context, issues []string) (SGSSCHistoryVerification, error) {
	return fetchSGSSCVerifiedHistoryWithRequests(ctx, issues, time.Now, rand.Reader, request163Mirror, requestSGSSCJSON)
}

// This is a historical-data fetcher, not another live poll. It neither requires
// 24 consecutive periods nor returns next-period metadata. It only returns the
// requested past issues and never changes live source health or scheduling.
// An attempt has at most five fixed-host GETs: the 163 latest+finite-history
// pair, one 115 identity probe, then 115 history for one or two issue dates.
// Existing HTTP deadlines, cancellation, response-size limits and redirect
// denial apply unchanged.
func fetchSGSSCVerifiedHistoryWithRequest(ctx context.Context, issues []string, now func() time.Time, request sgSSCRequest) (SGSSCHistoryVerification, error) {
	return fetchSGSSCVerifiedHistoryWithRequests(ctx, issues, now, rand.Reader, source163MirrorRequest(request), request)
}

func fetchSGSSCVerifiedHistoryWithRequests(ctx context.Context, issues []string, now func() time.Time, entropy io.Reader, request163 source163MirrorRequest, request115 sgSSCRequest) (SGSSCHistoryVerification, error) {
	if ctx == nil || now == nil || entropy == nil || request163 == nil || request115 == nil {
		return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩历史核对依赖不可用")
	}
	startedAt := now()
	targets, err := planSGSSCHistoryTargets(issues, startedAt)
	if err != nil {
		return SGSSCHistoryVerification{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, sgSSCTotalTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return SGSSCHistoryVerification{}, err
	}
	validation := sgSSCValidationStation()
	var mother []sourceDraw
	var verifierProbe sourceDraw
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		var fetchErr error
		strictRequest := func(requestContext context.Context, endpoint string) ([]byte, error) {
			body, requestErr := request163(requestContext, endpoint)
			if requestErr == nil {
				if strictErr := validateSGSSCJSONDocument(body); strictErr != nil {
					return nil, fmt.Errorf("SG时时彩163:64母源JSON无效: %w", strictErr)
				}
			}
			return body, requestErr
		}
		mother, fetchErr = fetch163MirrorDrawsWithRequest(groupContext, sgSSC163MotherBinding, func() time.Time { return startedAt }, entropy, strictRequest)
		if fetchErr != nil {
			return fmt.Errorf("SG时时彩163:64母源历史窗口: %w", fetchErr)
		}
		return nil
	})
	group.Go(func() error {
		body, requestErr := request115(groupContext, sgSSCEndpoint(validation, ""))
		if requestErr != nil {
			return fmt.Errorf("SG时时彩115校验源历史身份读取: %w", requestErr)
		}
		if len(body) == 0 || len(body) > sgSSCMaxResponseSize {
			return fmt.Errorf("SG时时彩115校验源响应为空或超过大小限制")
		}
		_, verifierProbe, requestErr = parseSGSSCIdentifiedLatest(body, validation)
		if requestErr != nil {
			return fmt.Errorf("SG时时彩115校验源历史身份核验: %w", requestErr)
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		return SGSSCHistoryVerification{}, err
	}
	identityCheckedAt := now()
	if identityCheckedAt.IsZero() || identityCheckedAt.Before(startedAt) {
		return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩历史核对期间本机时钟无效或回退")
	}
	if verifierProbe.DrawAt.After(identityCheckedAt) {
		return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩115校验源身份记录超前于本机时间")
	}
	historyURLs := make([]string, 0, len(targets.dates))
	for _, date := range targets.dates {
		historyURLs = append(historyURLs, sgSSCEndpoint(validation, date))
	}
	bodies, err := requestSGSSCBatch(ctx, historyURLs, request115)
	if err != nil {
		return SGSSCHistoryVerification{}, err
	}
	checkedAt := now()
	if checkedAt.IsZero() || checkedAt.Before(identityCheckedAt) {
		return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩历史核对期间本机时钟无效或回退")
	}
	motherHistory := make(map[string]sourceDraw, len(mother))
	for _, row := range mother {
		if _, duplicate := motherHistory[row.Issue]; duplicate {
			return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩163:64母源历史期号%s重复", row.Issue)
		}
		motherHistory[row.Issue] = row
	}
	verifierHistory := make(map[string]sourceDraw)
	for dateIndex, date := range targets.dates {
		// The 115 identity probe is not a live-health assertion. Raw history
		// must nevertheless never contain a future draw.
		rows, parseErr := parseSGSSCDatedHistory(bodies[dateIndex], validation, date, checkedAt, true)
		if parseErr != nil {
			return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩115校验源历史%s: %w", date, parseErr)
		}
		for _, row := range rows {
			if _, duplicate := verifierHistory[row.Issue]; duplicate {
				return SGSSCHistoryVerification{}, fmt.Errorf("SG时时彩115校验源历史期号%s重复", row.Issue)
			}
			verifierHistory[row.Issue] = row
		}
	}
	result := SGSSCHistoryVerification{Draws: make([]sourceDraw, 0, len(targets.issues)), Failures: make([]SGSSCHistoryFailure, 0)}
	for _, issue := range targets.issues {
		first, firstFound := motherHistory[issue]
		second, secondFound := verifierHistory[issue]
		if !firstFound || !secondFound {
			missing := "163母源和115校验源"
			if firstFound {
				missing = "115校验源"
			} else if secondFound {
				missing = "163母源有限历史"
			}
			result.Failures = append(result.Failures, SGSSCHistoryFailure{
				Issue: issue, Error: "SG时时彩" + missing + "历史缺少该期，保留待核对",
				Permanent: !firstFound,
			})
			continue
		}
		if !sameSGSSCResult(first, second) {
			result.Failures = append(result.Failures, SGSSCHistoryFailure{Issue: issue, Error: "SG时时彩该期163母源与115校验源五球或时间不一致，保留待核对"})
			continue
		}
		first.Numbers = append([]int(nil), first.Numbers...)
		first.SourceRevision, first.ConversionRevision = sgSSCSourceRevision, sgSSCConversionRevision
		result.Draws = append(result.Draws, first)
	}
	if err := ctx.Err(); err != nil {
		return SGSSCHistoryVerification{}, err
	}
	if err := validateSGSSCVerifiedHistoryBatch(result.Draws, targets.issues, checkedAt); err != nil {
		return SGSSCHistoryVerification{}, err
	}
	return result, nil
}

func planSGSSCHistoryTargets(issues []string, now time.Time) (sgSSCHistoryTargets, error) {
	if now.IsZero() || len(issues) == 0 || len(issues) > sgSSCBackfillMaxIssues {
		return sgSSCHistoryTargets{}, fmt.Errorf("SG时时彩历史核对需要有效本机时间及1至%d个期号", sgSSCBackfillMaxIssues)
	}
	plan := sgSSCHistoryTargets{issues: append([]string(nil), issues...), at: make(map[string]time.Time, len(issues))}
	dates := map[string]bool{}
	for _, issue := range issues {
		day, _, drawAt, err := parseSGSSCIssue(issue)
		if err != nil {
			return sgSSCHistoryTargets{}, err
		}
		if _, duplicate := plan.at[issue]; duplicate {
			return sgSSCHistoryTargets{}, fmt.Errorf("SG时时彩历史核对期号%s重复", issue)
		}
		if !drawAt.Before(now) || drawAt.Before(now.Add(-sgSSCBackfillMaxAge)) {
			return sgSSCHistoryTargets{}, fmt.Errorf("SG时时彩历史核对期号%s必须已过开奖时间且在最近30天内", issue)
		}
		plan.at[issue] = drawAt
		dates[day.Format("2006-01-02")] = true // 00:00 belongs to the previous day's 288.
	}
	if len(dates) > sgSSCBackfillMaxDates {
		return sgSSCHistoryTargets{}, fmt.Errorf("SG时时彩单批历史核对最多%d个期号日期", sgSSCBackfillMaxDates)
	}
	for date := range dates {
		plan.dates = append(plan.dates, date)
	}
	sort.Strings(plan.issues)
	sort.Strings(plan.dates)
	return plan, nil
}

// Validate only the successful subset; absent/disagreeing targets are reported
// separately and must remain unresolved. Empty success is valid. This is not
// a replacement for the database's immutable-source/legacy-ticket safeguards.
func validateSGSSCVerifiedHistoryBatch(draws []sourceDraw, targets []string, now time.Time) error {
	plan, err := planSGSSCHistoryTargets(targets, now)
	if err != nil {
		return err
	}
	if len(draws) > len(plan.issues) {
		return fmt.Errorf("SG时时彩历史成功记录超出请求范围")
	}
	for index, draw := range draws {
		expectedAt, requested := plan.at[draw.Issue]
		if !requested || !draw.DrawAt.Equal(expectedAt) || len(draw.Numbers) != 5 ||
			draw.SourceRevision != sgSSCSourceRevision || draw.ConversionRevision != sgSSCConversionRevision {
			return fmt.Errorf("SG时时彩历史成功记录第%d条不符合请求期号、原始记录或版本", index+1)
		}
		if draw.NextIssue != "" || !draw.NextDrawAt.IsZero() || draw.HasBingoSourceTail || draw.BingoOrderVerified || draw.BingoSourceTail != 0 {
			return fmt.Errorf("SG时时彩历史成功记录不得携带实时排期或其他彩种转换元数据")
		}
		for _, number := range draw.Numbers {
			if number < 0 || number > 9 {
				return fmt.Errorf("SG时时彩历史成功记录第%d条号码越界", index+1)
			}
		}
		if index > 0 && !draw.DrawAt.After(draws[index-1].DrawAt) {
			return fmt.Errorf("SG时时彩历史成功记录重复或未按时间升序排列")
		}
	}
	return nil
}
