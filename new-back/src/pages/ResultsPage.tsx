import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  FormControl,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import RefreshRounded from "@mui/icons-material/RefreshRounded";
import DownloadRounded from "@mui/icons-material/DownloadRounded";
import SearchRounded from "@mui/icons-material/SearchRounded";
import CloudSyncRounded from "@mui/icons-material/CloudSyncRounded";
import AccessTimeRounded from "@mui/icons-material/AccessTimeRounded";
import AutorenewRounded from "@mui/icons-material/AutorenewRounded";
import { useEffect, useMemo, useState } from "react";
import {
  adminApi,
  type AdminGame,
  type DrawResult,
  type FeedStatus,
} from "../api";
import { PageHeader } from "../components/PageHeader";
import { useFeedback } from "../components/feedback";
import { useServerClock } from "../hooks/useServerClock";

const formatTime = (value: string) =>
  new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value));
const formatClock = (value: number) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      }).format(new Date(value))
    : "正在校准服务器时间";
const countdown = (value: string | undefined, now: number) => {
  if (!value || !now) return "等待调度";
  const seconds = Math.max(
    0,
    Math.ceil((new Date(value).getTime() - now) / 1000),
  );
  return `${seconds} 秒后检查`;
};

function drawSummary(numbers: number[], gameId: string) {
  const sum = numbers.reduce((total, item) => total + item, 0);
  const thresholds: Record<string, number> = {
    "official-fc3d": 14,
    "official-pl3": 14,
    "official-qxc": 32,
    "official-tw-bingo": 810,
  };
  const threshold = thresholds[gameId];
  return {
    sum,
    size: threshold === undefined ? "—" : sum >= threshold ? "大" : "小",
    parity: threshold === undefined ? "—" : sum % 2 ? "单" : "双",
  };
}

const TARGET_GAME_IDS = new Set([
  'bingo-ssc-1', 'bingo-ssc-2', 'bingo-ssc-3', 'bingo-ssc-4',
  'bingo-racing-a', 'bingo-racing-b', 'bingo-mark-six',
  'hong-kong-mark-six', 'happy8-mark-six', 'new-macau-mark-six', 'old-macau-mark-six',
])

export function ResultsPage() {
  const [games, setGames] = useState<AdminGame[]>([]);
  const [gameId, setGameId] = useState("");
  const [draws, setDraws] = useState<DrawResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [syncing, setSyncing] = useState(false);
  const [syncingTarget, setSyncingTarget] = useState(false);
  const [settlingIssue, setSettlingIssue] = useState("");
  const [feedStatus, setFeedStatus] = useState<FeedStatus | null>(null);
  const { now, synced: clockSynced, latency } = useServerClock();
  const { showMessage } = useFeedback();
  useEffect(() => {
    Promise.all([adminApi.games(), adminApi.feedStatus()])
      .then(([next, status]) => {
        setGames(next);
        setFeedStatus(status);
        setGameId(
          next.find((game) => game.source_kind === "official")?.id ??
            next[0]?.id ??
            "",
        );
      })
      .catch((reason) => {
        setError(reason instanceof Error ? reason.message : "读取失败");
        setLoading(false);
      });
  }, []);
  useEffect(() => {
    if (!gameId) return;
    Promise.resolve()
      .then(() => setLoading(true))
      .then(() => adminApi.draws(gameId))
      .then(setDraws)
      .catch((reason) =>
        setError(reason instanceof Error ? reason.message : "读取失败"),
      )
      .finally(() => setLoading(false));
  }, [gameId]);
  useEffect(() => {
    if (!gameId) return;
    const refreshLive = () =>
      Promise.all([
        adminApi.draws(gameId),
        adminApi.games(),
        adminApi.feedStatus(),
      ])
        .then(([nextDraws, nextGames, status]) => {
          setDraws(nextDraws);
          setGames(nextGames);
          setFeedStatus(status);
        })
        .catch(() => undefined);
    const timer = window.setInterval(
      () => void refreshLive(),
      gameId === "official-tw-bingo" ? 5_000 : 10_000,
    );
    return () => window.clearInterval(timer);
  }, [gameId]);
  const filteredDraws = useMemo(
    () => draws.filter((draw) => draw.issue.includes(query.trim())),
    [draws, query],
  );
  const visibleDraws = filteredDraws.slice(
    page * rowsPerPage,
    page * rowsPerPage + rowsPerPage,
  );
  const targetReadyCount = games.filter((game) => TARGET_GAME_IDS.has(game.id)).length;
  const reload = async () => {
    if (!gameId) return;
    setLoading(true);
    setError("");
    try {
      setDraws(await adminApi.draws(gameId));
      showMessage("开奖结果已刷新");
    } catch {
      setError("刷新失败");
    } finally {
      setLoading(false);
    }
  };
  const exportCsv = () => {
    const game =
      games.find((item) => item.id === gameId)?.name ?? "开奖结果查询";
    const rows = filteredDraws.map((draw) => [
      draw.issue,
      formatTime(draw.draw_at),
      draw.numbers.join(" "),
      draw.numbers.reduce((sum, item) => sum + item, 0),
    ]);
    const csv = [["期号", "开奖时间", "开奖号码", "总和"], ...rows]
      .map((row) => row.join(","))
      .join("\n");
    const link = document.createElement("a");
    link.href = URL.createObjectURL(
      new Blob([`\uFEFF${csv}`], { type: "text/csv;charset=utf-8" }),
    );
    link.download = `${game}.csv`;
    link.click();
    URL.revokeObjectURL(link.href);
    showMessage("CSV 报表已导出");
  };
  const syncTargetGames = async () => {
    setSyncingTarget(true);
    setError("");
    try {
      const result = await adminApi.syncTargetGames();
      setGames(await adminApi.games());
      showMessage(
        result.created.length
          ? `已补全 ${result.created.length} 个目标彩种`
          : "目标 11 款彩种已齐全",
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "补全失败");
    } finally {
      setSyncingTarget(false);
    }
  };
  const syncOfficial = async () => {
    setSyncing(true);
    setError("");
    try {
      const response = await adminApi.syncOfficialSources();
      const [nextGames, status] = await Promise.all([
        adminApi.games(),
        adminApi.feedStatus(),
      ]);
      setGames(nextGames);
      setFeedStatus(status);
      if (gameId) setDraws(await adminApi.draws(gameId));
      const imported = response.results.reduce(
        (sum, item) => sum + item.imported,
        0,
      );
      showMessage(
        response.failed
          ? `同步完成，${response.failed} 个来源暂时失败`
          : `官方数据同步完成，新增 ${imported} 期`,
        response.failed ? "warning" : "success",
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "官方数据同步失败");
    } finally {
      setSyncing(false);
    }
  };
  const settleDraw = async (issue: string) => {
    if (!gameId) return;
    setSettlingIssue(issue);
    setError("");
    try {
      const result = await adminApi.settleIssue(gameId, issue);
      showMessage(
        `${result.issue} 结算完成：中 ${result.won} / 未中 ${result.lost}，派彩 ${result.payout_amount.toFixed(2)}`,
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "结算失败");
    } finally {
      setSettlingIssue("");
    }
  };
  const currentGame = games.find((game) => game.id === gameId);
  const currentJob = feedStatus?.jobs.find((job) =>
    job.game_ids.includes(gameId),
  );
  return (
    <Box p={{ xs: 2, lg: 2.5 }}>
      <PageHeader
        eyebrow="游戏运营 / 官方开奖"
        title="开奖结果查询"
        description={`自动跟随官方数据发布时间。目标彩种 ${targetReadyCount}/11 已入库。`}
        actions={
          <>
            <Button
              variant="outlined"
              startIcon={
                syncingTarget ? <CircularProgress size={16} /> : <AutorenewRounded />
              }
              disabled={syncingTarget}
              onClick={() => void syncTargetGames()}
            >
              补全目标彩种
            </Button>
            <Button
              variant="outlined"
              startIcon={<DownloadRounded />}
              disabled={!filteredDraws.length}
              onClick={exportCsv}
            >
              导出
            </Button>
            <Button
              variant="outlined"
              startIcon={
                syncing ? <CircularProgress size={16} /> : <CloudSyncRounded />
              }
              disabled={syncing}
              onClick={syncOfficial}
            >
              立即补抓
            </Button>
            <Button
              variant="contained"
              startIcon={
                loading ? (
                  <CircularProgress color="inherit" size={16} />
                ) : (
                  <RefreshRounded />
                )
              }
              disabled={loading}
              onClick={reload}
            >
              刷新
            </Button>
          </>
        }
      />
      <Card
        sx={{
          mt: 2,
          p: { xs: 1.25, md: 1.5 },
          width: "100%",
          maxWidth: 980,
          background: (theme) =>
            theme.palette.mode === "dark"
              ? "linear-gradient(135deg,rgba(20,108,143,.24),rgba(31,160,146,.12))"
              : "linear-gradient(135deg,#eefaff,#effcf9)",
        }}
      >
        <Stack
          direction={{ xs: "column", md: "row" }}
          alignItems={{ md: "center" }}
          gap={{ xs: 1.25, md: 2 }}
        >
          <Stack direction="row" alignItems="center" gap={1.2}>
            <Box
              sx={{
                width: 38,
                height: 38,
                borderRadius: 2.2,
                display: "grid",
                placeItems: "center",
                color: "#fff",
                bgcolor: "primary.main",
              }}
            >
              <AccessTimeRounded />
            </Box>
            <Box>
              <Typography
                fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
                fontSize={{ xs: 17, sm: 19 }}
                fontWeight={850}
                letterSpacing={0.5}
              >
                {formatClock(now)}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                北京时间 ·{" "}
                {clockSynced
                  ? `服务器已校准，网络 ${latency}ms`
                  : "正在与服务器校准"}
              </Typography>
            </Box>
          </Stack>
          <Box
            sx={{
              display: { xs: "none", md: "block" },
              width: "1px",
              height: 36,
              flex: "0 0 1px",
              bgcolor: "divider",
            }}
          />
          <Stack direction="row" alignItems="center" gap={1.2} flex={1}>
            <AutorenewRounded
              color={feedStatus?.running ? "success" : "disabled"}
            />
            <Box minWidth={0}>
              <Stack direction="row" alignItems="center" gap={1} flexWrap="wrap">
                <Typography fontWeight={800} noWrap>
                  {currentJob?.name ?? "官方开奖同步服务"}
                </Typography>
                <Chip
                  size="small"
                  color={
                    feedStatus?.running && !currentJob?.last_error
                      ? "success"
                      : "warning"
                  }
                  label={
                    currentJob?.running
                      ? "正在获取"
                      : feedStatus?.running
                        ? "自动同步中"
                        : "调度未启动"
                  }
                />
              </Stack>
              <Typography variant="caption" color="text.secondary">
                {currentJob?.mode === "draw-window"
                  ? "开奖窗口高频追踪"
                  : "常规巡检"}{" "}
                · {countdown(currentJob?.next_run_at, now)}
                {currentJob?.last_error ? ` · ${currentJob.last_error}` : ""}
              </Typography>
            </Box>
          </Stack>
        </Stack>
      </Card>
      <Card sx={{ mt: 1.5, p: { xs: 1.25, sm: 2 } }}>
        <Stack
          direction={{ xs: "column", md: "row" }}
          alignItems={{ md: "center" }}
          gap={1.25}
          mb={1.5}
        >
          <FormControl size="small" sx={{ minWidth: { xs: "100%", sm: 260 } }}>
            <InputLabel>选择游戏</InputLabel>
            <Select
              label="选择游戏"
              value={gameId}
              onChange={(event) => {
                setGameId(event.target.value);
                setPage(0);
              }}
            >
              {games.map((game) => (
                <MenuItem value={game.id} key={game.id}>
                  {game.source_kind === "official" ? "【官方】" : "【演示】"}
                  {game.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            placeholder="搜索期号"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setPage(0);
            }}
            slotProps={{
              input: {
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchRounded fontSize="small" />
                  </InputAdornment>
                ),
              },
            }}
            sx={{ minWidth: { xs: "100%", sm: 210 } }}
          />
          <Chip label={`${filteredDraws.length} 条记录`} variant="outlined" />
          <Typography
            variant="caption"
            color="text.secondary"
            ml={{ md: "auto" }}
          >
            当前期号：{currentGame?.issue ?? "—"}
          </Typography>
        </Stack>
        {currentGame?.source_kind === "official" && (
          <Alert
            severity={
              currentGame.sync_status === "error" ? "warning" : "success"
            }
            sx={{ mb: 1.5 }}
          >
            <Stack
              direction={{ xs: "column", sm: "row" }}
              gap={{ xs: 0.25, sm: 1 }}
            >
              <Typography component="span" variant="body2" fontWeight={750}>
                官方数据 · {currentGame.source_name}
              </Typography>
              <Typography component="span" variant="body2">
                {currentGame.last_sync_at
                  ? `最近同步：${formatTime(currentGame.last_sync_at)}`
                  : "等待首次同步"}
              </Typography>
              {currentGame.last_sync_error && (
                <Typography component="span" variant="body2">
                  {currentGame.last_sync_error}
                </Typography>
              )}
            </Stack>
          </Alert>
        )}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        {loading ? (
          <Stack alignItems="center" py={10}>
            <CircularProgress size={30} />
          </Stack>
        ) : (
          <>
            <TableContainer sx={{ maxHeight: "calc(100vh - 330px)" }}>
              <Table stickyHeader size="small" sx={{ minWidth: 720 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>期号</TableCell>
                    <TableCell>开奖时间</TableCell>
                    <TableCell>开奖号码</TableCell>
                    <TableCell align="center">总和</TableCell>
                    <TableCell align="center">大小</TableCell>
                    <TableCell align="center">单双</TableCell>
                    <TableCell align="right">操作</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {visibleDraws.map((draw) => {
                    const summary = drawSummary(draw.numbers, gameId);
                    return (
                      <TableRow hover key={draw.id}>
                        <TableCell sx={{ fontSize: 11, fontWeight: 700 }}>
                          {draw.issue}
                        </TableCell>
                        <TableCell sx={{ fontSize: 11 }}>
                          {formatTime(draw.draw_at)}
                        </TableCell>
                        <TableCell>
                          <Stack direction="row" gap={0.6}>
                            {draw.numbers.map((item, index) => (
                              <Box
                                key={`${draw.id}-${index}`}
                                sx={{
                                  width: 28,
                                  height: 28,
                                  flex: "0 0 auto",
                                  display: "grid",
                                  placeItems: "center",
                                  borderRadius: "50%",
                                  color: "#fff",
                                  fontSize: 11,
                                  fontWeight: 800,
                                  bgcolor: [
                                    "#6c8ef1",
                                    "#ee7080",
                                    "#3eb18c",
                                    "#df9f3b",
                                    "#8a70df",
                                  ][index % 5],
                                }}
                              >
                                {item}
                              </Box>
                            ))}
                          </Stack>
                        </TableCell>
                        <TableCell align="center">{summary.sum}</TableCell>
                        <TableCell align="center">
                          {summary.size === "—" ? (
                            "—"
                          ) : (
                            <Chip
                              size="small"
                              color={
                                summary.size === "大" ? "error" : "primary"
                              }
                              variant="outlined"
                              label={summary.size}
                            />
                          )}
                        </TableCell>
                        <TableCell align="center">
                          {summary.parity === "—" ? (
                            "—"
                          ) : (
                            <Chip
                              size="small"
                              variant="outlined"
                              label={summary.parity}
                            />
                          )}
                        </TableCell>
                        <TableCell align="right">
                          <Button
                            size="small"
                            variant="outlined"
                            disabled={settlingIssue === draw.issue}
                            onClick={() => void settleDraw(draw.issue)}
                          >
                            {settlingIssue === draw.issue ? "结算中…" : "结算"}
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                  {!visibleDraws.length && (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        align="center"
                        sx={{ py: 8, color: "text.secondary" }}
                      >
                        暂无匹配的开奖记录
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
            <TablePagination
              component="div"
              count={filteredDraws.length}
              page={page}
              onPageChange={(_, next) => setPage(next)}
              rowsPerPage={rowsPerPage}
              onRowsPerPageChange={(event) => {
                setRowsPerPage(Number(event.target.value));
                setPage(0);
              }}
              rowsPerPageOptions={[10, 20, 30]}
              labelRowsPerPage="每页"
            />
          </>
        )}
      </Card>
    </Box>
  );
}
