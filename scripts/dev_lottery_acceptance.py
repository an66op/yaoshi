#!/usr/bin/env python3
"""Local, post-reset lottery acceptance runner.

The runner is deliberately API-only: it never clears data, updates catalogue
configuration, or connects to PostgreSQL.  A secure business-reset receipt and
an authenticated member cookie are mandatory before it will submit anything.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import http.cookiejar
import json
import os
import re
import stat
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter
from decimal import Decimal, InvalidOperation, ROUND_HALF_UP
from pathlib import Path
from typing import Any


REQUEST_ID_RE = re.compile(r"^[A-Za-z0-9._:-]{8,96}$")
GAME_ID_RE = re.compile(r"^[A-Za-z0-9._-]+$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
MEMBER_COOKIE = "wangzhe_member_session"
BLOCKED_WORDS = ("封盘", "未开放", "暂停", "暂未", "规则", "赔率", "开奖", "期号", "数据源", "同步", "维护")


class Fatal(RuntimeError):
    pass


def secure_regular_file(path: Path, label: str) -> None:
    if not path.is_absolute():
        raise Fatal(f"{label}必须使用绝对路径")
    if path.is_symlink() or not path.is_file():
        raise Fatal(f"{label}必须是普通文件且不能是符号链接：{path}")
    info = path.stat()
    if info.st_uid != os.getuid():
        raise Fatal(f"{label}必须属于当前用户：{path}")
    if stat.S_IMODE(info.st_mode) & 0o077:
        raise Fatal(f"{label}权限过宽；请执行 chmod 600：{path}")


def secure_backup_artifact(path: Path, label: str) -> None:
    """Accept 0600 or the backup monitor's intentional read-only 0640."""
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise Fatal(f"{label}必须是绝对路径下的普通文件：{path}")
    info = path.stat()
    if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o027:
        raise Fatal(f"{label}owner 或权限不安全：{path}")


def parse_receipt(path: Path) -> dict[str, str]:
    secure_regular_file(path, "业务重置凭证")
    if not path.name.endswith(".reset-receipt") or path.name.endswith(".full-reset-receipt"):
        raise Fatal("只接受 dev-reset-business-data.sh 生成的 *.reset-receipt")
    values: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        if not raw.strip():
            continue
        if "=" not in raw:
            raise Fatal("业务重置凭证格式不正确")
        key, value = raw.split("=", 1)
        if key in values:
            raise Fatal(f"业务重置凭证字段重复：{key}")
        values[key] = value
    required = {
        "request_id", "database", "backup", "backup_sha256",
        "server_system_identifier", "server_address", "server_port",
        "sentinel_token_sha256", "completed_at_utc",
    }
    if set(values) != required:
        missing = sorted(required - set(values))
        extra = sorted(set(values) - required)
        raise Fatal(f"业务重置凭证字段不匹配（缺少={missing}，多余={extra}）")
    if not REQUEST_ID_RE.fullmatch(values["request_id"]):
        raise Fatal("业务重置凭证 request_id 不正确")
    if not SHA256_RE.fullmatch(values["backup_sha256"]) or not SHA256_RE.fullmatch(values["sentinel_token_sha256"]):
        raise Fatal("业务重置凭证 SHA-256 不正确")
    if Path(values["backup"]).name != values["backup"]:
        raise Fatal("业务重置凭证中的备份文件名不安全")
    backup = path.parent / values["backup"]
    checksum = Path(str(backup) + ".sha256")
    secure_backup_artifact(backup, "重置前备份")
    secure_backup_artifact(checksum, "备份校验文件")
    checksum_value = checksum.read_text(encoding="utf-8").split()[0]
    if checksum_value != values["backup_sha256"]:
        raise Fatal("业务重置凭证与备份校验文件不一致")
    try:
        dt.datetime.strptime(values["completed_at_utc"], "%Y-%m-%dT%H:%M:%SZ")
    except ValueError as exc:
        raise Fatal("业务重置凭证完成时间不正确") from exc
    return values


def validate_api_base(raw: str) -> str:
    value = raw.rstrip("/")
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in {"http", "https"} or parsed.hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise Fatal("验收工具只允许连接本机 127.0.0.1、localhost 或 ::1")
    if parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path.rstrip("/") != "/api":
        raise Fatal("--api-base 必须是无凭据、无查询参数的本机 /api 地址")
    return value


def decimal_value(value: Any, fallback: Decimal = Decimal("0")) -> Decimal:
    try:
        result = Decimal(str(value))
    except (InvalidOperation, ValueError):
        return fallback
    return result if result.is_finite() else fallback


def amount_text(value: Decimal) -> str:
    text = format(value.quantize(Decimal("0.01"), rounding=ROUND_HALF_UP), "f")
    return text.rstrip("0").rstrip(".") if "." in text else text


def cents(value: Any) -> int:
    return int((decimal_value(value) * 100).quantize(Decimal("1"), rounding=ROUND_HALF_UP))


class API:
    def __init__(self, base: str, cookie_path: Path, timeout: float) -> None:
        self.base = base
        self.cookie_path = cookie_path
        self.timeout = timeout
        self.jar = http.cookiejar.MozillaCookieJar(str(cookie_path))
        if cookie_path.exists() and cookie_path.stat().st_size:
            self._load_netscape_cookies(cookie_path)
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def _load_netscape_cookies(self, path: Path) -> None:
        """Load curl/Netscape cookies without dropping #HttpOnly_ records.

        Some Python releases treat curl's standard #HttpOnly_ prefix as an
        ordinary comment. Parse the seven-column format explicitly so a real
        curl cookie jar behaves the same as a Python-generated test jar.
        """
        expected_host = urllib.parse.urlsplit(self.base).hostname or ""
        try:
            rows = path.read_text(encoding="utf-8").splitlines()
        except (OSError, UnicodeDecodeError) as exc:
            raise Fatal("会员 cookie jar 无法读取") from exc
        for raw in rows:
            if not raw.strip():
                continue
            http_only = raw.startswith("#HttpOnly_")
            if raw.startswith("#") and not http_only:
                continue
            if http_only:
                raw = raw[len("#HttpOnly_"):]
            fields = raw.split("\t")
            if len(fields) != 7:
                raise Fatal("会员 cookie jar 不是标准 Netscape/curl 七列格式")
            domain, include_subdomains, cookie_path, secure_text, expires_text, name, value = fields
            if domain.lstrip(".").lower() != expected_host.lower():
                raise Fatal("会员 cookie jar 域名与本机 API 不匹配")
            if include_subdomains not in {"TRUE", "FALSE"} or secure_text not in {"TRUE", "FALSE"}:
                raise Fatal("会员 cookie jar 标志位不正确")
            if not cookie_path.startswith("/") or not expires_text.isdigit():
                raise Fatal("会员 cookie jar 路径或过期时间不正确")
            expires = int(expires_text)
            if expires and expires <= int(time.time()):
                continue
            if not re.fullmatch(r"[A-Za-z0-9_.-]{1,128}", name) or not value or any(char in value for char in "\x00\r\n\t"):
                raise Fatal("会员 cookie jar 名称或值不安全")
            self.jar.set_cookie(http.cookiejar.Cookie(
                version=0, name=name, value=value, port=None, port_specified=False,
                domain=domain, domain_specified=include_subdomains == "TRUE",
                domain_initial_dot=domain.startswith("."), path=cookie_path,
                path_specified=True, secure=secure_text == "TRUE",
                expires=expires or None, discard=expires == 0,
                comment=None, comment_url=None,
                rest={"HTTPOnly": ""} if http_only else {}, rfc2109=False,
            ))

    def save(self) -> None:
        self.jar.save(ignore_discard=True, ignore_expires=True)
        os.chmod(self.cookie_path, 0o600)

    def has_member_cookie(self) -> bool:
        return any(cookie.name == MEMBER_COOKIE and bool(cookie.value) for cookie in self.jar)

    def request(self, method: str, path: str, payload: dict[str, Any] | None = None) -> tuple[int, dict[str, Any]]:
        body = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base + path, data=body, headers=headers, method=method)
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                status = response.status
                raw = response.read()
        except urllib.error.HTTPError as exc:
            status, raw = exc.code, exc.read()
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise Fatal(f"本地 API 不可用：{exc}") from exc
        try:
            parsed = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise Fatal(f"API 返回非 JSON（HTTP {status}）") from exc
        if not isinstance(parsed, dict):
            raise Fatal(f"API 返回结构不正确（HTTP {status}）")
        return status, parsed


def envelope_data(status: int, body: dict[str, Any], expected: int) -> dict[str, Any]:
    if status != expected or body.get("code") != expected or not isinstance(body.get("data"), dict):
        raise Fatal(str(body.get("message") or f"HTTP {status}"))
    return body["data"]


def valid_odds_item(item: dict[str, Any], show_odds: bool) -> bool:
    if not isinstance(item.get("play_code"), str):
        return False
    if show_odds and decimal_value(item.get("odds")) <= 1:
        return False
    amount = max(decimal_value(item.get("min_bet")), Decimal("0.01"))
    maximum = decimal_value(item.get("max_bet"))
    period = decimal_value(item.get("max_user_period"))
    return (maximum <= 0 or amount <= maximum) and (period <= 0 or amount <= period)


def stake_for(item: dict[str, Any], requested: Decimal) -> Decimal | None:
    amount = max(requested, decimal_value(item.get("min_bet")), Decimal("0.01"))
    maximum = decimal_value(item.get("max_bet"))
    period = decimal_value(item.get("max_user_period"))
    if (maximum > 0 and amount > maximum) or (period > 0 and amount > period):
        return None
    return amount


def assistant_payload(odds: dict[str, Any], requested: Decimal) -> tuple[str, int] | None:
    show = odds.get("show_odds") is not False
    items = [item for item in odds.get("items", []) if isinstance(item, dict) and valid_odds_item(item, show)]
    for code, selections in (("ball_1_5", "12"), ("two_sided", "大小")):
        item = next((row for row in items if row.get("play_code") == code), None)
        if item is None:
            continue
        amount = stake_for(item, requested)
        if amount is None:
            continue
        period = decimal_value(item.get("max_user_period"))
        if period > 0 and amount * 2 > period:
            continue
        return f"1/{selections}/{amount_text(amount)}", 2
    shape: list[tuple[str, str, Decimal]] = []
    for code, label in (("leopard", "豹子"), ("straight", "顺子"), ("pair", "对子"), ("half_straight", "半顺"), ("mixed", "杂六")):
        item = next((row for row in items if row.get("play_code") == code), None)
        if item is not None and (amount := stake_for(item, requested)) is not None:
            shape.append((code, label, amount))
    if len(shape) >= 2:
        return "#".join(f"前三/{label}/{amount_text(amount)}" for _, label, amount in shape[:2]), 2
    return None


def web_candidate(item: dict[str, Any], selection: str, position: int, amount: Decimal) -> dict[str, Any]:
    return {
        "play_code": item["play_code"], "play_name": str(item.get("play_name") or ""),
        "position": position, "selection": selection, "amount": float(amount),
    }


def pc_candidates(item: dict[str, Any], amount: Decimal) -> list[dict[str, Any]]:
    code = item["play_code"]
    exact = re.fullmatch(r"pc28_sum_exact_(\d+)_(\d+)", code)
    if exact:
        values = [exact.group(1)]
        if exact.group(2) != exact.group(1):
            values.append(exact.group(2))
        return [web_candidate(item, value, 0, amount) for value in values]
    mapping: dict[str, list[tuple[str, int]]] = {
        "pc28_package_three": [("0,1,2", 0), ("3,4,5", 0)],
        "pc28_position_number": [("0", 1), ("1", 2)],
        "pc28_position_two_sided": [("大", 1), ("小", 2)],
        "pc28_dragon_tiger": [("龙", 1), ("虎", 1)],
        "pc28_dragon_tiger_tie": [("和", 1)],
        "pc28_sum_size": [("大", 0), ("小", 0)],
        "pc28_sum_parity": [("单", 0), ("双", 0)],
        "pc28_combo_big_odd": [("大单", 0)], "pc28_combo_big_even": [("大双", 0)],
        "pc28_combo_small_odd": [("小单", 0)], "pc28_combo_small_even": [("小双", 0)],
        "pc28_extreme": [("极大", 0), ("极小", 0)],
        "pc28_color_red": [("红波", 0)], "pc28_color_green": [("绿波", 0)],
        "pc28_color_blue": [("蓝波", 0)], "pc28_leopard": [("豹子", 0)],
        "pc28_pair": [("对子", 0)], "pc28_straight": [("顺子", 0)],
    }
    return [web_candidate(item, selection, position, amount) for selection, position in mapping.get(code, [])]


def marksix_candidates(item: dict[str, Any], amount: Decimal) -> list[dict[str, Any]]:
    code = item["play_code"]
    mapping: dict[str, list[tuple[str, int]]] = {
        "marksix_special_a_number": [("1", 7), ("2", 7)],
        "marksix_special_b_number": [("3", 7), ("4", 7)],
        "marksix_regular_number": [("5", 0), ("6", 0)],
        "marksix_regular_position_number": [("7", 1), ("8", 2)],
        "marksix_regular_special_number": [("9", 1), ("10", 2)],
        "marksix_combo_4_all": [("1,2,3,4", 0)], "marksix_combo_3_all": [("1,2,3", 0)],
        "marksix_combo_2_all": [("1,2", 0)], "marksix_combo_special_pair": [("1,49", 0)],
        "marksix_not_in": [("1,2,3,4,5", 0)], "marksix_combo_3_2": [("1,2,3", 0)],
        "marksix_combo_2_special": [("1,49", 0)],
        "marksix_special_big_small": [("大", 7), ("小", 7)],
        "marksix_special_odd_even": [("单", 7), ("双", 7)],
        "marksix_special_sum_big_small": [("合大", 7), ("合小", 7)],
        "marksix_special_sum_odd_even": [("合单", 7), ("合双", 7)],
        "marksix_total_odd_even": [("总和单", 0), ("总和双", 0)],
        "marksix_total_big_small": [("总和大", 0), ("总和小", 0)],
        "marksix_special_half": [("大单", 7), ("小双", 7)],
    }
    return [web_candidate(item, selection, position, amount) for selection, position in mapping.get(code, [])]


def web_payload(odds: dict[str, Any], requested: Decimal, family: str) -> list[dict[str, Any]] | None:
    show = odds.get("show_odds") is not False
    candidates: list[dict[str, Any]] = []
    for item in odds.get("items", []):
        if not isinstance(item, dict) or not valid_odds_item(item, show):
            continue
        amount = stake_for(item, requested)
        if amount is None:
            continue
        generated = pc_candidates(item, amount) if family == "pc28" else marksix_candidates(item, amount)
        period = decimal_value(item.get("max_user_period"))
        for candidate in generated:
            same_code_count = sum(1 for existing in candidates if existing["play_code"] == candidate["play_code"])
            if period > 0 and amount * (same_code_count + 1) > period:
                continue
            key = (candidate["play_code"], candidate["position"], candidate["selection"])
            if all((row["play_code"], row["position"], row["selection"]) != key for row in candidates):
                candidates.append(candidate)
            if len(candidates) == 2:
                return candidates
    return None


def list_total(api: API, game_id: str, issue: str) -> int:
    query = urllib.parse.urlencode({"game_id": game_id, "issue": issue, "status": "all", "page": 1, "page_size": 1})
    status, body = api.request("GET", "/member/bets?" + query)
    data = envelope_data(status, body, 200)
    return int(data.get("total", -1))


def list_total_all_issues(api: API, game_id: str) -> int:
    query = urllib.parse.urlencode({"game_id": game_id, "status": "all", "page": 1, "page_size": 1})
    status, body = api.request("GET", "/member/bets?" + query)
    data = envelope_data(status, body, 200)
    return int(data.get("total", -1))


def list_all_member_bets(api: API) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    seen_ids: set[int] = set()
    expected_total: int | None = None
    page = 1
    while True:
        query = urllib.parse.urlencode({"game_id": "all", "status": "all", "page": page, "page_size": 100})
        status, body = api.request("GET", "/member/bets?" + query)
        data = envelope_data(status, body, 200)
        items, total = data.get("items"), data.get("total")
        if not isinstance(items, list) or type(total) is not int or total < 0:
            raise Fatal("注单列表响应缺少有效的 items/total")
        if total > 10000:
            raise Fatal("验收账号注单数量异常，拒绝续测")
        if expected_total is None:
            expected_total = total
        elif total != expected_total:
            raise Fatal("读取续测账号注单期间数据发生变化")
        for item in items:
            if not isinstance(item, dict) or type(item.get("id")) is not int or not isinstance(item.get("game_id"), str):
                raise Fatal("注单列表包含结构不正确的记录")
            row_id = int(item["id"])
            if row_id in seen_ids:
                raise Fatal("注单列表分页出现重复记录")
            seen_ids.add(row_id)
            rows.append(item)
        if len(rows) >= total:
            break
        if not items or data.get("has_more") is False:
            raise Fatal("注单列表分页未返回完整记录")
        page += 1
    if expected_total is None or len(rows) != expected_total:
        raise Fatal("注单列表总数与 items 不一致")
    return rows


def bet_ledger_ids(api: API) -> set[int]:
    status, body = api.request("GET", "/member/balance-history?limit=50")
    data = envelope_data(status, body, 200)
    items = data.get("items")
    if not isinstance(items, list):
        raise Fatal("账变响应缺少 items")
    return {int(item["id"]) for item in items if isinstance(item, dict) and item.get("type") == "bet" and isinstance(item.get("id"), int)}


def result_row(game: dict[str, Any], state: str, stage: str, reason: str, **extra: Any) -> dict[str, Any]:
    row = {
        "game_id": game.get("id", ""), "game_name": game.get("name", ""),
        "state": state, "stage": stage, "reason": reason,
    }
    row.update(extra)
    return row


def acceptance_request_id(reset_request: str, game_id: str) -> str:
    return "accept:" + hashlib.sha256(f"{reset_request}:{game_id}".encode()).hexdigest()[:32]


def load_blocked_continuation(path: Path, receipt: dict[str, str]) -> tuple[dict[str, Any], dict[str, int], set[str]]:
    secure_regular_file(path, "上一轮验收报告")
    if path.stat().st_size > 5 * 1024 * 1024:
        raise Fatal("上一轮验收报告过大")
    try:
        report = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise Fatal("上一轮验收报告无法读取或格式不正确") from exc
    if not isinstance(report, dict) or report.get("schema_version") != 1:
        raise Fatal("上一轮验收报告版本不正确")
    if report.get("reset_request_id") != receipt["request_id"] or report.get("database") != receipt["database"]:
        raise Fatal("上一轮验收报告与本次业务重置凭证不匹配")
    if report.get("catalog_mismatch") is not False:
        raise Fatal("上一轮验收报告存在彩种目录异常，不能续测")
    results, counts = report.get("results"), report.get("counts")
    if not isinstance(results, list) or not isinstance(counts, dict):
        raise Fatal("上一轮验收报告缺少 results/counts")
    normalized_counts = {state: sum(1 for row in results if isinstance(row, dict) and row.get("state") == state) for state in ("pass", "blocked", "fail")}
    if set(counts) != {"pass", "blocked", "fail"} or any(type(counts[state]) is not int for state in counts) or counts != normalized_counts:
        raise Fatal("上一轮验收报告 counts 与 results 不一致")
    if normalized_counts["fail"] or any(not isinstance(row, dict) or row.get("state") not in {"pass", "blocked"} for row in results):
        raise Fatal("上一轮验收报告含 FAIL 或未知状态，不能续测")
    pass_counts: dict[str, int] = {}
    blocked_ids: set[str] = set()
    seen_games: set[str] = set()
    for row in results:
        game_id = str(row.get("game_id") or "")
        if not GAME_ID_RE.fullmatch(game_id) or game_id in seen_games:
            raise Fatal("上一轮验收报告包含无效或重复的彩种 ID")
        seen_games.add(game_id)
        if row["state"] == "blocked":
            blocked_ids.add(game_id)
            continue
        bet_count = row.get("bet_count")
        if row.get("stage") != "complete" or row.get("first_submit") != "new" or type(bet_count) is not int or bet_count < 2:
            raise Fatal(f"上一轮 PASS 回执不完整：{game_id}")
        if row.get("request_id") != acceptance_request_id(receipt["request_id"], game_id):
            raise Fatal(f"上一轮 PASS 请求标识不匹配：{game_id}")
        pass_counts[game_id] = bet_count
    if not blocked_ids:
        raise Fatal("上一轮验收报告没有 BLOCKED 彩种")
    return report, pass_counts, blocked_ids


def response_reason(status: int, body: dict[str, Any]) -> tuple[str, str]:
    message = str(body.get("message") or f"HTTP {status}")
    if status == 401:
        raise Fatal("会员登录已失效，请重新生成 cookie jar")
    if status == 400 and any(word in message for word in BLOCKED_WORDS) and "余额" not in message:
        return "blocked", message
    return "fail", message


def test_game(api: API, game: dict[str, Any], reset_request: str, requested_stake: Decimal) -> dict[str, Any]:
    game_id, game_name = str(game.get("id") or ""), str(game.get("name") or "")
    if not GAME_ID_RE.fullmatch(game_id):
        return result_row(game, "fail", "catalog", "彩种 ID 格式不安全")
    encoded = urllib.parse.quote(game_id, safe="")
    try:
        status_code, status_body = api.request("GET", f"/member/games/{encoded}/assistant")
        if status_code != 200 or status_body.get("code") != 200 or not isinstance(status_body.get("data"), dict):
            state, reason = response_reason(status_code, status_body)
            return result_row(game, state, "status", reason)
        status = status_body["data"]
        if status.get("rules_ready") is not True:
            return result_row(game, "blocked", "rules", str(status.get("rules_message") or "玩法规则未就绪"))
        if status.get("accepting") is not True:
            reason = str(status.get("source_error") or f"不受注（issue_status={status.get('issue_status')}, source_healthy={status.get('source_healthy')}）")
            return result_row(game, "blocked", "source_or_window", reason, issue=status.get("issue", ""))
        issue = str(status.get("issue") or "")
        if not issue:
            return result_row(game, "blocked", "issue", "当前没有可受注期号")

        odds_code, odds_body = api.request("GET", f"/member/games/{encoded}/odds")
        if odds_code != 200 or odds_body.get("code") != 200 or not isinstance(odds_body.get("data"), dict):
            state, reason = response_reason(odds_code, odds_body)
            return result_row(game, state, "odds", reason, issue=issue)
        odds = odds_body["data"]
        if odds.get("rules_ready") is not True:
            return result_row(game, "blocked", "odds", str(odds.get("rules_message") or "赔率规则未就绪"), issue=issue)
        version = str(odds.get("rule_version") or status.get("rule_version") or "")
        modes = odds.get("bet_modes") if isinstance(odds.get("bet_modes"), dict) else {}
        family = "marksix" if "mark6" in version else "pc28" if version.startswith("pc28-") else "assistant"
        request_id = acceptance_request_id(reset_request, game_id)
        if family in {"pc28", "marksix"}:
            if modes.get("web") is not True:
                return result_row(game, "blocked", "bet_mode", "详细网投未开启", issue=issue, rule_version=version)
            items = web_payload(odds, requested_stake, family)
            if not items:
                return result_row(game, "blocked", "odds", "找不到两个可验证且赔率有效的网投项", issue=issue, rule_version=version)
            payload = {"issue": issue, "items": items, "request_id": request_id}
            path = f"/member/games/{encoded}/web-bets"
        else:
            if modes.get("chat") is not True:
                return result_row(game, "blocked", "bet_mode", "聊天投注未开启", issue=issue, rule_version=version)
            generated = assistant_payload(odds, requested_stake)
            if generated is None:
                return result_row(game, "blocked", "odds", "找不到两个可验证且赔率有效的助手投注项", issue=issue, rule_version=version)
            content, _ = generated
            payload = {"issue": issue, "content": content, "request_id": request_id}
            path = f"/member/games/{encoded}/assistant/bets"

        before_total = list_total(api, game_id, issue)
        before_ledgers = bet_ledger_ids(api)
        first_code, first_body = api.request("POST", path, payload)
        if first_code != 201 or first_body.get("code") != 201 or not isinstance(first_body.get("data"), dict):
            state, reason = response_reason(first_code, first_body)
            return result_row(game, state, "submit", reason, issue=issue, rule_version=version)
        first = first_body["data"]
        lines = first.get("lines")
        if not isinstance(lines, list) or int(first.get("bet_count", 0)) != len(lines) or len(lines) < 2:
            return result_row(game, "fail", "receipt", "受理回执不足两注或数量不一致", issue=issue, rule_version=version)
        keys = {(line.get("play_code"), line.get("position"), line.get("selection")) for line in lines if isinstance(line, dict)}
        if len(keys) != len(lines) or any(decimal_value(line.get("odds")) <= 1 for line in lines if isinstance(line, dict)):
            return result_row(game, "fail", "receipt", "回执含重复项或无效赔率", issue=issue, rule_version=version)
        after_first_total = list_total(api, game_id, issue)
        after_first_ledgers = bet_ledger_ids(api)
        bet_delta = after_first_total - before_total
        ledger_delta = after_first_ledgers - before_ledgers
        if bet_delta != len(lines):
            return result_row(game, "fail", "atomicity", f"首次请求注单增量异常：{bet_delta}", issue=issue, rule_version=version)
        if len(ledger_delta) != 1:
            return result_row(game, "fail", "ledger", f"首次请求账变增量异常：{len(ledger_delta)}", issue=issue, rule_version=version)

        replay_code, replay_body = api.request("POST", path, payload)
        if replay_code != 201 or replay_body.get("code") != 201 or not isinstance(replay_body.get("data"), dict):
            state, reason = response_reason(replay_code, replay_body)
            return result_row(game, state, "idempotency_replay", reason, issue=issue, rule_version=version)
        replay = replay_body["data"]
        after_replay_total = list_total(api, game_id, issue)
        after_replay_ledgers = bet_ledger_ids(api)
        if after_replay_total != after_first_total or (after_replay_ledgers - after_first_ledgers):
            return result_row(game, "fail", "idempotency", "同一 request_id 重放产生了新注单或新扣款", issue=issue, rule_version=version)
        if replay != first or cents(replay.get("balance")) != cents(first.get("balance")):
            return result_row(game, "fail", "idempotency", "同一 request_id 未返回完全相同的冻结回执", issue=issue, rule_version=version)
        return result_row(
            game, "pass", "complete", "多注原子受理及幂等重放通过", issue=issue,
            rule_version=version, request_id=request_id, bet_count=len(lines),
            total=first.get("total"), balance=first.get("balance"),
            first_submit="new",
        )
    except Fatal as exc:
        if "登录已失效" in str(exc):
            raise
        return result_row(game, "fail", "api", str(exc))


def secure_output_path(raw: str | None) -> Path:
    if raw:
        path = Path(raw)
        if not path.is_absolute():
            raise Fatal("--report 必须使用绝对路径")
        if path.exists():
            secure_regular_file(path, "验收报告")
        else:
            parent = path.parent
            if not parent.is_dir() or parent.is_symlink():
                raise Fatal("验收报告父目录不存在或不安全")
        return path
    descriptor, name = tempfile.mkstemp(prefix="wangzhe-lottery-acceptance-", dir=os.environ.get("TMPDIR") or "/tmp")
    os.close(descriptor)
    os.chmod(name, 0o600)
    return Path(name)


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="本机业务重置后逐彩种多注验收（不会清数据或改配置）")
    parser.add_argument("--reset-receipt", required=True, help="业务重置生成的绝对 *.reset-receipt")
    parser.add_argument("--cookie-jar", required=True, help="owner-only 的会员 cookie jar 绝对路径")
    parser.add_argument("--login-json", help="可选 owner-only 登录 JSON；必须含当前有效图片验证码")
    parser.add_argument("--api-base", default="http://127.0.0.1:8080/api")
    parser.add_argument("--stake", default="1", help="每项最低测试金额；实际会提升到后台 min_bet")
    parser.add_argument("--expect-games", type=int, default=30, help="预期会员可见彩种数；0 表示不校验")
    parser.add_argument("--game", action="append", default=[], help="只测试指定彩种，可重复；目录数量仍会校验")
    parser.add_argument(
        "--continue-blocked-from",
        help="只续测同一业务重置凭证下、上一轮报告中 BLOCKED 的彩种（报告必须为 owner-only 绝对路径）",
    )
    parser.add_argument("--delay", type=float, default=0.05, help="彩种之间等待秒数")
    parser.add_argument("--timeout", type=float, default=12.0, help="单次 HTTP 超时秒数")
    parser.add_argument("--report", help="JSON 报告绝对路径；默认写入系统临时目录")
    return parser.parse_args()


def main() -> int:
    args = arguments()
    started = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    receipt = parse_receipt(Path(args.reset_receipt))
    base = validate_api_base(args.api_base)
    continuation_path: Path | None = None
    continuation_report: dict[str, Any] | None = None
    previous_pass_counts: dict[str, int] = {}
    previous_blocked: set[str] = set()
    if args.continue_blocked_from:
        continuation_path = Path(args.continue_blocked_from)
        continuation_report, previous_pass_counts, previous_blocked = load_blocked_continuation(continuation_path, receipt)
        requested = set(args.game)
        invalid_targets = sorted(requested - previous_blocked)
        if invalid_targets:
            raise Fatal(f"续测目标必须全部来自上一轮 BLOCKED：{', '.join(invalid_targets)}")
        selected = requested or set(previous_blocked)
    else:
        selected = set(args.game)
    try:
        stake = Decimal(args.stake)
    except InvalidOperation as exc:
        raise Fatal("--stake 必须是正数，最多两位小数") from exc
    if not stake.is_finite() or stake <= 0 or stake.as_tuple().exponent < -2:
        raise Fatal("--stake 必须是正数，最多两位小数")
    if args.expect_games < 0 or args.delay < 0 or args.timeout <= 0:
        raise Fatal("数量、等待或超时参数不正确")

    cookie_path = Path(args.cookie_jar)
    if args.login_json:
        login_path = Path(args.login_json)
        secure_regular_file(login_path, "登录凭据文件")
        if cookie_path.exists():
            secure_regular_file(cookie_path, "会员 cookie jar")
        else:
            if not cookie_path.is_absolute() or cookie_path.parent.is_symlink() or not cookie_path.parent.is_dir():
                raise Fatal("新 cookie jar 必须使用安全父目录下的绝对路径")
            cookie_path.touch(mode=0o600, exist_ok=False)
        api = API(base, cookie_path, args.timeout)
        try:
            login = json.loads(login_path.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise Fatal("登录 JSON 无法读取或格式不正确") from exc
        if not isinstance(login, dict) or set(login) != {"username", "password", "captcha_id", "captcha_code"}:
            raise Fatal("登录 JSON 只能包含 username、password、captcha_id、captcha_code")
        if any(not isinstance(login.get(key), str) or not login[key] for key in ("username", "password", "captcha_id")):
            raise Fatal("登录 JSON 的 username、password、captcha_id 必须是非空字符串")
        if not re.fullmatch(r"\d{6}", str(login.get("captcha_code") or "")):
            raise Fatal("登录 JSON 必须提供当前有效的 6 位图片验证码")
        try:
            code, body = api.request("POST", "/member/login", login)
        finally:
            login.clear()
        if code != 200 or body.get("code") != 200:
            raise Fatal(str(body.get("message") or f"会员登录失败（HTTP {code}）"))
        api.save()
    else:
        secure_regular_file(cookie_path, "会员 cookie jar")
        api = API(base, cookie_path, args.timeout)
    if not api.has_member_cookie():
        raise Fatal("cookie jar 中没有 wangzhe_member_session；请先正常完成验证码登录")

    me_code, me_body = api.request("GET", "/member/me")
    envelope_data(me_code, me_body, 200)
    wallet_code, wallet_body = api.request("GET", "/member/wallet/summary")
    wallet = envelope_data(wallet_code, wallet_body, 200)
    wallet_total = wallet.get("total_bet_count")
    if type(wallet_total) is not int or wallet_total < 0:
        raise Fatal("钱包统计缺少有效的 total_bet_count")
    if continuation_report is None:
        if wallet_total != 0:
            raise Fatal("该会员已有注单；为避免跨期续跑或重复扣款，请使用业务重置后的空白验收账号")
    else:
        for game_id in sorted(selected):
            target_total = list_total_all_issues(api, game_id)
            if target_total < 0:
                raise Fatal(f"无法确认续测目标的历史注单：{game_id}")
            if target_total != 0:
                raise Fatal(f"续测目标彩种已有注单，拒绝跨期或重复验收：{game_id}")
        existing_bets = list_all_member_bets(api)
        if len(existing_bets) != wallet_total:
            raise Fatal("钱包注单总数与注单列表不一致，拒绝续测")
        actual_counts = Counter(str(row["game_id"]) for row in existing_bets)
        unexpected_games = sorted(set(actual_counts) - set(previous_pass_counts))
        if unexpected_games:
            raise Fatal(f"账号含非上一轮 PASS 彩种的既有注单：{', '.join(unexpected_games)}")
        if dict(actual_counts) != previous_pass_counts:
            raise Fatal("账号既有注单数量与上一轮 PASS 回执不一致，拒绝续测")

    games_code, games_body = api.request("GET", "/member/games")
    if games_code != 200 or games_body.get("code") != 200 or not isinstance(games_body.get("data"), list):
        raise Fatal(str(games_body.get("message") or "读取会员彩种失败"))
    games = [game for game in games_body["data"] if isinstance(game, dict)]
    games.sort(key=lambda row: (int(row.get("lobby_sort_order") or 0), str(row.get("id") or "")))
    catalog_mismatch = bool(args.expect_games and len(games) != args.expect_games)
    missing_selected = sorted(selected - {str(game.get("id") or "") for game in games})
    if continuation_report is not None and catalog_mismatch:
        raise Fatal(f"续测时彩种目录数量不匹配：预期 {args.expect_games}，实际 {len(games)}")
    if continuation_report is not None and missing_selected:
        raise Fatal(f"上一轮 BLOCKED 彩种已不在当前会员目录：{', '.join(missing_selected)}")
    if selected:
        games = [game for game in games if str(game.get("id") or "") in selected]

    report_path = secure_output_path(args.report)
    if continuation_path is not None and report_path.resolve() == continuation_path.resolve():
        raise Fatal("--report 不能覆盖 --continue-blocked-from 的上一轮报告")
    run_results: list[dict[str, Any]] = []
    print(f"业务重置：{receipt['request_id']} · 会员可见彩种：{len(games_body['data'])}")
    if catalog_mismatch:
        print(f"[FAIL] 目录数量：预期 {args.expect_games}，实际 {len(games_body['data'])}")
    for missing in missing_selected:
        run_results.append({"game_id": missing, "game_name": "", "state": "fail", "stage": "catalog", "reason": "会员目录中不存在指定彩种"})
    for index, game in enumerate(games, 1):
        print(f"[{index}/{len(games)}] {game.get('name')} ({game.get('id')})", flush=True)
        row = test_game(api, game, receipt["request_id"], stake)
        run_results.append(row)
        print(f"  [{row['state'].upper()}] {row['stage']} · {row['reason']}", flush=True)
        if args.delay:
            time.sleep(args.delay)
    api.save()

    if continuation_report is not None:
        replacements = {str(row["game_id"]): row for row in run_results}
        results = [replacements.get(str(row["game_id"]), row) for row in continuation_report["results"]]
    else:
        results = run_results
    run_counts = {state: sum(1 for row in run_results if row["state"] == state) for state in ("pass", "blocked", "fail")}
    counts = {state: sum(1 for row in results if row["state"] == state) for state in ("pass", "blocked", "fail")}
    completed = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    report = {
        "schema_version": 1, "reset_request_id": receipt["request_id"], "database": receipt["database"],
        "api_base": base, "started_at": started, "completed_at": completed,
        "expected_game_count": args.expect_games, "visible_game_count": len(games_body["data"]),
        "catalog_mismatch": catalog_mismatch, "counts": counts, "results": results,
        "run_counts": run_counts,
    }
    if continuation_path is not None:
        report["continued_from"] = str(continuation_path)
    temporary = report_path.with_name(report_path.name + ".partial")
    if temporary.exists() or temporary.is_symlink():
        raise Fatal(f"报告临时文件已存在，拒绝覆盖：{temporary}")
    temporary.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    os.chmod(temporary, 0o600)
    os.replace(temporary, report_path)
    if continuation_report is not None:
        print(
            f"本轮：PASS={run_counts['pass']} BLOCKED={run_counts['blocked']} FAIL={run_counts['fail']} · "
            f"累计：PASS={counts['pass']} BLOCKED={counts['blocked']} FAIL={counts['fail']} · 报告：{report_path}"
        )
    else:
        print(f"汇总：PASS={counts['pass']} BLOCKED={counts['blocked']} FAIL={counts['fail']} · 报告：{report_path}")
    if catalog_mismatch or counts["fail"]:
        return 1
    return 2 if counts["blocked"] else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Fatal as error:
        print(f"验收停止：{error}", file=sys.stderr)
        raise SystemExit(1)
