#!/usr/bin/env python3
from __future__ import annotations

import copy
import hashlib
import http.cookiejar
import http.server
import json
import os
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.parse
from pathlib import Path
from typing import Any


SCRIPT = Path(__file__).resolve().parents[1] / "dev-lottery-acceptance.sh"


class AcceptanceAPI(http.server.ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address: tuple[str, int]) -> None:
        super().__init__(address, AcceptanceHandler)
        self.balance = 1000.0
        self.requests: dict[str, tuple[dict[str, Any], dict[str, Any]]] = {}
        self.game_counts = {"speed-racing": 0, "canada-28": 0, "source-blocked": 0}
        self.bets: list[dict[str, Any]] = []
        self.next_bet = 1
        self.ledger: list[dict[str, Any]] = []
        self.next_ledger = 1
        self.posts = {"speed-racing": 0, "canada-28": 0, "source-blocked": 0}
        self.blocked_accepting = False


class AcceptanceHandler(http.server.BaseHTTPRequestHandler):
    server: AcceptanceAPI

    def log_message(self, _format: str, *_args: Any) -> None:
        return

    def send_json(self, status: int, data: Any = None, message: str = "ok") -> None:
        payload: dict[str, Any] = {"code": status, "message": message}
        if data is not None:
            payload["data"] = data
        raw = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def authenticated(self) -> bool:
        return "wangzhe_member_session=fake-member-session" in self.headers.get("Cookie", "")

    def do_GET(self) -> None:
        if not self.authenticated():
            self.send_json(401, message="登录已失效")
            return
        parsed = urllib.parse.urlsplit(self.path)
        path = parsed.path
        if path == "/api/member/me":
            self.send_json(200, {"id": 88, "balance": self.server.balance})
            return
        if path == "/api/member/wallet/summary":
            self.send_json(200, {"total_bet_count": sum(self.server.game_counts.values())})
            return
        if path == "/api/member/games":
            games = [
                {"id": "speed-racing", "name": "极速赛车", "lobby_sort_order": 1},
                {"id": "canada-28", "name": "加拿大28", "lobby_sort_order": 2},
                {"id": "source-blocked", "name": "来源异常彩种", "lobby_sort_order": 3},
            ]
            self.send_json(200, games)
            return
        if path.startswith("/api/member/games/") and path.endswith("/assistant"):
            game = urllib.parse.unquote(path.split("/")[-2])
            if game == "source-blocked" and not self.server.blocked_accepting:
                self.send_json(200, {
                    "game_id": game, "rules_ready": True, "rule_version": "racing-v2",
                    "accepting": False, "issue": "3", "issue_status": "error",
                    "source_healthy": False, "source_error": "母源过期",
                })
                return
            version = "pc28-v2" if game == "canada-28" else "racing-v2"
            self.send_json(200, {
                "game_id": game, "rules_ready": True, "rule_version": version,
                "accepting": True, "issue": "1001", "issue_status": "accepting", "source_healthy": True,
            })
            return
        if path.startswith("/api/member/games/") and path.endswith("/odds"):
            game = urllib.parse.unquote(path.split("/")[-2])
            if game == "canada-28":
                items = [{
                    "play_code": "pc28_sum_exact_0_27", "play_name": "和值0/27", "odds": 50,
                    "min_bet": 1, "max_bet": 100, "max_user_period": 1000,
                }]
                version, modes = "pc28-v2", {"chat": True, "web": True}
            else:
                items = [{
                    "play_code": "ball_1_5", "play_name": "指定名次号码", "odds": 9,
                    "min_bet": 1, "max_bet": 100, "max_user_period": 1000,
                }]
                version, modes = "racing-v2", {"chat": True, "web": True}
            self.send_json(200, {
                "rules_ready": True, "rule_version": version, "bet_modes": modes,
                "show_odds": True, "items": items,
            })
            return
        if path == "/api/member/bets":
            query = urllib.parse.parse_qs(parsed.query)
            game = query.get("game_id", ["all"])[0]
            issue = query.get("issue", [""])[0]
            page = int(query.get("page", ["1"])[0])
            page_size = int(query.get("page_size", ["20"])[0])
            rows = [
                row for row in reversed(self.server.bets)
                if (game in {"", "all"} or row["game_id"] == game) and (not issue or row["issue"] == issue)
            ]
            start = (page - 1) * page_size
            items = rows[start:start + page_size]
            self.send_json(200, {
                "items": items, "total": len(rows), "page": page, "page_size": page_size,
                "has_more": start + len(items) < len(rows), "next_before_id": items[-1]["id"] if items else 0,
            })
            return
        if path == "/api/member/balance-history":
            self.send_json(200, {"items": list(reversed(self.server.ledger)), "has_more": False, "next_before_id": 0})
            return
        self.send_json(404, message="不存在")

    def do_POST(self) -> None:
        if not self.authenticated():
            self.send_json(401, message="登录已失效")
            return
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length))
        path = urllib.parse.urlsplit(self.path).path
        if not (path.endswith("/assistant/bets") or path.endswith("/web-bets")):
            self.send_json(404, message="不存在")
            return
        parts = path.split("/")
        game = urllib.parse.unquote(parts[-2] if path.endswith("/web-bets") else parts[-3])
        self.server.posts[game] += 1
        request_id = payload["request_id"]
        previous = self.server.requests.get(request_id)
        if previous:
            if previous[0] != payload:
                self.send_json(400, message="幂等冲突")
                return
            self.send_json(201, previous[1], "已受理")
            return
        if game != "canada-28":
            lines = [
                {"play_code": "ball_1_5", "play_name": "指定名次号码", "position": 1, "selection": "1", "amount": 1, "odds": 9},
                {"play_code": "ball_1_5", "play_name": "指定名次号码", "position": 1, "selection": "2", "amount": 1, "odds": 9},
            ]
        else:
            lines = [
                {"play_code": item["play_code"], "play_name": item["play_name"], "position": item["position"],
                 "selection": item["selection"], "amount": item["amount"], "odds": 50}
                for item in payload["items"]
            ]
        total = sum(float(item["amount"]) for item in lines)
        self.server.balance -= total
        self.server.game_counts[game] += len(lines)
        for item in lines:
            self.server.bets.append({
                "id": self.server.next_bet, "game_id": game, "issue": payload["issue"],
                "play_code": item["play_code"], "selection": item["selection"], "status": "pending",
            })
            self.server.next_bet += 1
        self.server.ledger.append({
            "id": self.server.next_ledger, "type": "bet", "amount": -total,
            "before": self.server.balance + total, "after": self.server.balance,
        })
        self.server.next_ledger += 1
        result = {
            "game_id": game, "game_name": game, "issue": payload["issue"],
            "content": "test", "lines": lines, "bet_count": len(lines), "total": total,
            "balance": self.server.balance, "accepted_at": "2026-09-05T00:00:00Z",
            "rule_version": "pc28-v2" if game == "canada-28" else "racing-v2",
        }
        self.server.requests[request_id] = (payload, result)
        self.send_json(201, result, "已受理")


def secure_write(path: Path, content: str) -> None:
    path.write_text(content, encoding="utf-8")
    path.chmod(0o600)


def fixture_files(root: Path) -> tuple[Path, Path]:
    backup = root / "business.sql.gz"
    secure_write(backup, "fixture backup\n")
    digest = hashlib.sha256(backup.read_bytes()).hexdigest()
    secure_write(Path(str(backup) + ".sha256"), f"{digest}  {backup.name}\n")
    receipt = root / "business.sql.gz.reset-receipt"
    secure_write(receipt, "\n".join([
        "request_id=dev-reset-20260905T000000Z-test-1234",
        "database=wangzhe_test",
        f"backup={backup.name}",
        f"backup_sha256={digest}",
        "server_system_identifier=1234567890",
        "server_address=127.0.0.1",
        "server_port=5432",
        f"sentinel_token_sha256={'a' * 64}",
        "completed_at_utc=2026-09-05T00:00:00Z",
        "",
    ]))
    cookie_path = root / "member.cookies"
    jar = http.cookiejar.MozillaCookieJar(str(cookie_path))
    jar.set_cookie(http.cookiejar.Cookie(
        version=0, name="wangzhe_member_session", value="fake-member-session", port=None,
        port_specified=False, domain="127.0.0.1", domain_specified=False, domain_initial_dot=False,
        path="/api", path_specified=True, secure=False, expires=int(time.time()) + 3600,
        discard=False, comment=None, comment_url=None, rest={"HttpOnly": None}, rfc2109=False,
    ))
    jar.save(ignore_discard=True, ignore_expires=True)
    # Match the jar curl writes for the real HttpOnly member session. Older
    # Python cookiejar versions otherwise skip this line as a comment.
    cookie_lines = cookie_path.read_text(encoding="utf-8").splitlines()
    cookie_path.write_text("\n".join(
        "#HttpOnly_" + line if line and not line.startswith("#") else line
        for line in cookie_lines
    ) + "\n", encoding="utf-8")
    cookie_path.chmod(0o600)
    return receipt, cookie_path


class AcceptanceRunnerTest(unittest.TestCase):
    def run_initial(self, root: Path, server: AcceptanceAPI, receipt: Path, cookie: Path) -> tuple[subprocess.CompletedProcess[str], Path]:
        report = root / "initial-report.json"
        result = subprocess.run([
            str(SCRIPT), "--reset-receipt", str(receipt), "--cookie-jar", str(cookie),
            "--api-base", f"http://127.0.0.1:{server.server_port}/api", "--expect-games", "3",
            "--delay", "0", "--report", str(report),
        ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
        return result, report

    def continue_from(
        self, root: Path, server: AcceptanceAPI, receipt: Path, cookie: Path, source: Path,
        name: str = "continued-report.json", games: list[str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        command = [
            str(SCRIPT), "--reset-receipt", str(receipt), "--cookie-jar", str(cookie),
            "--api-base", f"http://127.0.0.1:{server.server_port}/api", "--expect-games", "3",
            "--delay", "0", "--continue-blocked-from", str(source), "--report", str(root / name),
        ]
        for game in games or []:
            command.extend(["--game", game])
        return subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)

    def test_multi_line_web_and_assistant_replay_and_blocked_source(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            report = root / "report.json"
            server = AcceptanceAPI(("127.0.0.1", 0))
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                result = subprocess.run([
                    str(SCRIPT), "--reset-receipt", str(receipt), "--cookie-jar", str(cookie),
                    "--api-base", f"http://127.0.0.1:{server.server_port}/api", "--expect-games", "3",
                    "--delay", "0", "--report", str(report),
                ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
            payload = json.loads(report.read_text(encoding="utf-8"))
            self.assertEqual(payload["counts"], {"pass": 2, "blocked": 1, "fail": 0})
            self.assertEqual(server.posts, {"speed-racing": 2, "canada-28": 2, "source-blocked": 0})
            self.assertEqual(server.game_counts, {"speed-racing": 2, "canada-28": 2, "source-blocked": 0})
            self.assertEqual(len(server.ledger), 2)

    def test_continue_only_previous_blocked_game(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            server = AcceptanceAPI(("127.0.0.1", 0))
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                initial, report = self.run_initial(root, server, receipt, cookie)
                self.assertEqual(initial.returncode, 2, initial.stdout + initial.stderr)
                server.blocked_accepting = True
                result = self.continue_from(root, server, receipt, cookie, report)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            payload = json.loads((root / "continued-report.json").read_text(encoding="utf-8"))
            self.assertEqual(payload["counts"], {"pass": 3, "blocked": 0, "fail": 0})
            self.assertEqual(payload["run_counts"], {"pass": 1, "blocked": 0, "fail": 0})
            self.assertEqual(server.posts, {"speed-racing": 2, "canada-28": 2, "source-blocked": 2})
            self.assertEqual(server.game_counts, {"speed-racing": 2, "canada-28": 2, "source-blocked": 2})
            self.assertEqual(len(server.ledger), 3)

    def test_continue_rejects_mismatched_failed_or_nonblocked_report_target(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            server = AcceptanceAPI(("127.0.0.1", 0))
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                initial, report = self.run_initial(root, server, receipt, cookie)
                self.assertEqual(initial.returncode, 2, initial.stdout + initial.stderr)
                original = json.loads(report.read_text(encoding="utf-8"))

                mismatch = copy.deepcopy(original)
                mismatch["reset_request_id"] = "dev-reset-20260905T000000Z-other-1234"
                mismatch_path = root / "mismatch.json"
                secure_write(mismatch_path, json.dumps(mismatch))
                mismatch_result = self.continue_from(root, server, receipt, cookie, mismatch_path, "mismatch-output.json")

                failed = copy.deepcopy(original)
                blocked = next(row for row in failed["results"] if row["state"] == "blocked")
                blocked["state"] = "fail"
                failed["counts"] = {"pass": 2, "blocked": 0, "fail": 1}
                failed_path = root / "failed.json"
                secure_write(failed_path, json.dumps(failed))
                failed_result = self.continue_from(root, server, receipt, cookie, failed_path, "failed-output.json")

                nonblocked_result = self.continue_from(
                    root, server, receipt, cookie, report, "nonblocked-output.json", ["speed-racing"],
                )
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            self.assertEqual(mismatch_result.returncode, 1)
            self.assertIn("不匹配", mismatch_result.stderr)
            self.assertEqual(failed_result.returncode, 1)
            self.assertIn("含 FAIL", failed_result.stderr)
            self.assertEqual(nonblocked_result.returncode, 1)
            self.assertIn("必须全部来自上一轮 BLOCKED", nonblocked_result.stderr)
            self.assertEqual(server.posts, {"speed-racing": 2, "canada-28": 2, "source-blocked": 0})

    def test_continue_rejects_target_with_existing_bet_in_any_issue(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            server = AcceptanceAPI(("127.0.0.1", 0))
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                initial, report = self.run_initial(root, server, receipt, cookie)
                self.assertEqual(initial.returncode, 2, initial.stdout + initial.stderr)
                server.bets.append({
                    "id": server.next_bet, "game_id": "source-blocked", "issue": "older-issue",
                    "play_code": "ball_1_5", "selection": "1", "status": "pending",
                })
                server.next_bet += 1
                server.game_counts["source-blocked"] = 1
                server.blocked_accepting = True
                result = self.continue_from(root, server, receipt, cookie, report)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            self.assertIn("续测目标彩种已有注单", result.stderr)
            self.assertEqual(server.posts["source-blocked"], 0)

    def test_rejects_non_loopback_api_before_using_cookie(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            result = subprocess.run([
                str(SCRIPT), "--reset-receipt", str(receipt), "--cookie-jar", str(cookie),
                "--api-base", "https://example.invalid/api",
            ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
            self.assertEqual(result.returncode, 1)
            self.assertIn("只允许连接本机", result.stderr)

    def test_rejects_nonempty_member_before_any_submission(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt, cookie = fixture_files(root)
            server = AcceptanceAPI(("127.0.0.1", 0))
            server.game_counts["speed-racing"] = 1
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                result = subprocess.run([
                    str(SCRIPT), "--reset-receipt", str(receipt), "--cookie-jar", str(cookie),
                    "--api-base", f"http://127.0.0.1:{server.server_port}/api", "--expect-games", "3",
                    "--delay", "0",
                ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            self.assertIn("该会员已有注单", result.stderr)
            self.assertEqual(server.posts, {"speed-racing": 0, "canada-28": 0, "source-blocked": 0})


if __name__ == "__main__":
    unittest.main()
