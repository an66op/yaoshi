--
-- PostgreSQL database dump
--

-- Dumped from database version 17.4
-- Dumped by pg_dump version 17.4

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = "heap";

--
-- Name: activity_participations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."activity_participations" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "activity_id" bigint NOT NULL,
    "action" character varying(30) NOT NULL,
    "biz_date" character varying(10) NOT NULL,
    "reference" character varying(80) DEFAULT ''::character varying NOT NULL,
    "reward_cents" bigint DEFAULT 0 NOT NULL,
    "streak" bigint DEFAULT 0 NOT NULL,
    "participated_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone,
    CONSTRAINT "chk_activity_participation_financials" CHECK ((("reward_cents" >= 0) AND ("streak" >= 0)))
);


--
-- Name: activity_participations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."activity_participations_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: activity_participations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."activity_participations_id_seq" OWNED BY "public"."activity_participations"."id";


--
-- Name: admin_audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."admin_audit_logs" (
    "id" bigint NOT NULL,
    "event_id" character varying(96) DEFAULT ''::character varying NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "actor_id" bigint NOT NULL,
    "actor_name" character varying(80) NOT NULL,
    "actor_role" character varying(20) NOT NULL,
    "room_scope" character varying(64),
    "method" character varying(10) NOT NULL,
    "path" character varying(240) NOT NULL,
    "target_ref" character varying(240) DEFAULT ''::character varying NOT NULL,
    "status_code" bigint NOT NULL,
    "request_id" character varying(96),
    "ip" character varying(80),
    "created_at" timestamp with time zone
);


--
-- Name: admin_audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."admin_audit_logs_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_audit_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."admin_audit_logs_id_seq" OWNED BY "public"."admin_audit_logs"."id";


--
-- Name: admin_notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."admin_notifications" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "title" character varying(120) NOT NULL,
    "content" character varying(500) NOT NULL,
    "level" character varying(20) DEFAULT 'info'::character varying NOT NULL,
    "link" character varying(120),
    "read" boolean DEFAULT false NOT NULL,
    "created_at" timestamp with time zone,
    "read_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "deleted_by" character varying(80) DEFAULT ''::character varying NOT NULL,
    "cleanup_request_id" character varying(96) DEFAULT ''::character varying NOT NULL
);


--
-- Name: admin_notifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."admin_notifications_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."admin_notifications_id_seq" OWNED BY "public"."admin_notifications"."id";


--
-- Name: agent_profit_share_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."agent_profit_share_records" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "biz_date" character varying(10) NOT NULL,
    "agent_id" bigint NOT NULL,
    "room_scope" character varying(80) NOT NULL,
    "agent_username" character varying(50) NOT NULL,
    "room_code" character varying(40),
    "bet_count" bigint DEFAULT 0 NOT NULL,
    "turnover_cents" bigint DEFAULT 0 NOT NULL,
    "payout_cents" bigint DEFAULT 0 NOT NULL,
    "gross_profit_cents" bigint DEFAULT 0 NOT NULL,
    "rebate_cents" bigint DEFAULT 0 NOT NULL,
    "accrued_share_cents" bigint DEFAULT 0 NOT NULL,
    "paid_share_cents" bigint DEFAULT 0 NOT NULL,
    "last_transaction_id" bigint,
    "run_count" bigint DEFAULT 0 NOT NULL,
    "status" character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "operator" character varying(80),
    "last_paid_at" timestamp with time zone,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    CONSTRAINT "chk_agent_profit_share_financials" CHECK ((("bet_count" >= 0) AND ("turnover_cents" >= 0) AND ("payout_cents" >= 0) AND ("rebate_cents" >= 0) AND ("accrued_share_cents" >= 0) AND ("paid_share_cents" >= 0) AND ("paid_share_cents" <= "accrued_share_cents") AND ("run_count" >= 0) AND ("btrim"(("room_scope")::"text") <> ''::"text") AND (("status")::"text" = ANY ((ARRAY['pending'::character varying, 'credited'::character varying])::"text"[]))))
);


--
-- Name: agent_profit_share_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."agent_profit_share_records_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: agent_profit_share_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."agent_profit_share_records_id_seq" OWNED BY "public"."agent_profit_share_records"."id";


--
-- Name: chat_red_packet_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."chat_red_packet_claims" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "packet_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "amount_cents" bigint NOT NULL,
    "created_at" timestamp with time zone
);


--
-- Name: chat_red_packet_claims_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."chat_red_packet_claims_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_red_packet_claims_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."chat_red_packet_claims_id_seq" OWNED BY "public"."chat_red_packet_claims"."id";


--
-- Name: chat_red_packets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."chat_red_packets" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "message_id" bigint NOT NULL,
    "scope" character varying(64) NOT NULL,
    "room_scope" character varying(64) NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "funding_user_id" bigint DEFAULT 0 NOT NULL,
    "total_cents" bigint NOT NULL,
    "remaining_cents" bigint NOT NULL,
    "refunded_cents" bigint DEFAULT 0 NOT NULL,
    "packet_count" bigint NOT NULL,
    "claimed_count" bigint DEFAULT 0 NOT NULL,
    "min_daily_turnover_cents" bigint DEFAULT 0 NOT NULL,
    "greeting" character varying(60) NOT NULL,
    "cover" character varying(24) NOT NULL,
    "status" character varying(20) DEFAULT 'active'::character varying NOT NULL,
    "expires_at" timestamp with time zone,
    "closed_at" timestamp with time zone,
    "closed_by" character varying(80) DEFAULT ''::character varying NOT NULL,
    "close_reason" character varying(240) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: chat_red_packets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."chat_red_packets_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_red_packets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."chat_red_packets_id_seq" OWNED BY "public"."chat_red_packets"."id";


--
-- Name: entertainment_platforms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."entertainment_platforms" (
    "id" bigint NOT NULL,
    "code" character varying(40) NOT NULL,
    "name" character varying(80) NOT NULL,
    "category" character varying(40) NOT NULL,
    "merchant_no" character varying(120),
    "api_base" character varying(320),
    "launch_path" character varying(320),
    "secret_key" "text",
    "status" character varying(20) DEFAULT 'disabled'::character varying NOT NULL,
    "remark" character varying(500),
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: entertainment_platforms_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."entertainment_platforms_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: entertainment_platforms_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."entertainment_platforms_id_seq" OWNED BY "public"."entertainment_platforms"."id";


--
-- Name: lottery_assistant_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_assistant_requests" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "request_id" character varying(96) NOT NULL,
    "result_json" "text",
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: lottery_assistant_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_assistant_requests_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_assistant_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_assistant_requests_id_seq" OWNED BY "public"."lottery_assistant_requests"."id";


--
-- Name: lottery_bet_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_bet_requests" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "request_id" character varying(96) NOT NULL,
    "status" character varying(20) DEFAULT 'processing'::character varying NOT NULL,
    "result_json" "text",
    "last_error" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: lottery_bet_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_bet_requests_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_bet_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_bet_requests_id_seq" OWNED BY "public"."lottery_bet_requests"."id";


--
-- Name: lottery_bets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_bets" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "issue" character varying(64) NOT NULL,
    "room_scope" character varying(64) DEFAULT 'legacy'::character varying NOT NULL,
    "user_id" bigint NOT NULL,
    "username" character varying(50) NOT NULL,
    "play_code" character varying(40) NOT NULL,
    "play_name" character varying(40) NOT NULL,
    "position" bigint DEFAULT 0 NOT NULL,
    "selection" character varying(40) NOT NULL,
    "amount_cents" bigint NOT NULL,
    "odds" numeric DEFAULT 1.993 NOT NULL,
    "status" character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "payout_cents" bigint DEFAULT 0 NOT NULL,
    "fly_cents" bigint DEFAULT 0 NOT NULL,
    "rebate_rate_snapshot" numeric DEFAULT 0 NOT NULL,
    "rebate_cents" bigint DEFAULT 0 NOT NULL,
    "agent_share_rate_snapshot" numeric DEFAULT 0 NOT NULL,
    "agent_share_cents" bigint DEFAULT 0 NOT NULL,
    "settled_at" timestamp with time zone,
    "remark" character varying(300),
    "operator" character varying(80),
    "reconciliation_status" character varying(24) DEFAULT 'normal'::character varying NOT NULL,
    "reconciliation_note" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    CONSTRAINT "chk_lottery_bet_financials" CHECK ((("amount_cents" > 0) AND ("odds" > (0)::numeric) AND ("payout_cents" >= 0) AND ("fly_cents" >= 0) AND ("rebate_cents" >= 0) AND ("agent_share_cents" >= 0) AND (("rebate_rate_snapshot" >= (0)::numeric) AND ("rebate_rate_snapshot" <= (100)::numeric)) AND (("agent_share_rate_snapshot" >= (0)::numeric) AND ("agent_share_rate_snapshot" <= (100)::numeric)))),
    CONSTRAINT "chk_lottery_bet_status" CHECK (((("status")::"text" = ANY ((ARRAY['pending'::character varying, 'won'::character varying, 'lost'::character varying, 'cancelled'::character varying])::"text"[])) AND (("reconciliation_status")::"text" = ANY ((ARRAY['normal'::character varying, 'abnormal'::character varying, 'resolved'::character varying])::"text"[]))))
);


--
-- Name: lottery_bets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_bets_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_bets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_bets_id_seq" OWNED BY "public"."lottery_bets"."id";


--
-- Name: lottery_draws; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_draws" (
    "id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "issue" character varying(64) NOT NULL,
    "numbers" character varying(120) NOT NULL,
    "draw_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone
);


--
-- Name: lottery_draws_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_draws_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_draws_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_draws_id_seq" OWNED BY "public"."lottery_draws"."id";


--
-- Name: lottery_games; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_games" (
    "id" character varying(40) NOT NULL,
    "code" character varying(40) NOT NULL,
    "name" character varying(80) NOT NULL,
    "category" character varying(40) NOT NULL,
    "lobby_category" character varying(40) DEFAULT ''::character varying NOT NULL,
    "lobby_sort_order" bigint DEFAULT 0 NOT NULL,
    "badge" character varying(24) NOT NULL,
    "badge_color" character varying(24) NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "draw_interval" bigint DEFAULT 300 NOT NULL,
    "next_draw_at" timestamp with time zone,
    "source_kind" character varying(20) DEFAULT 'platform'::character varying NOT NULL,
    "source_name" character varying(80),
    "source_url" character varying(320),
    "sync_status" character varying(20) DEFAULT 'idle'::character varying NOT NULL,
    "last_sync_at" timestamp with time zone,
    "last_sync_error" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: lottery_issues; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_issues" (
    "id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "issue" character varying(64) NOT NULL,
    "status" character varying(24) DEFAULT 'pending'::character varying NOT NULL,
    "source_mode" character varying(20) DEFAULT 'platform'::character varying NOT NULL,
    "accept_at" timestamp with time zone NOT NULL,
    "seal_at" timestamp with time zone NOT NULL,
    "draw_at" timestamp with time zone,
    "settled_at" timestamp with time zone,
    "last_error" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: lottery_issues_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_issues_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_issues_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_issues_id_seq" OWNED BY "public"."lottery_issues"."id";


--
-- Name: lottery_lobby_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_lobby_categories" (
    "id" bigint NOT NULL,
    "name" character varying(40) NOT NULL,
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone
);


--
-- Name: lottery_lobby_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_lobby_categories_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_lobby_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_lobby_categories_id_seq" OWNED BY "public"."lottery_lobby_categories"."id";


--
-- Name: lottery_play_limits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."lottery_play_limits" (
    "id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "play_code" character varying(40) NOT NULL,
    "play_name" character varying(40) NOT NULL,
    "odds" numeric DEFAULT 1.993 NOT NULL,
    "min_bet" numeric DEFAULT 1 NOT NULL,
    "max_bet" numeric DEFAULT 50000 NOT NULL,
    "max_user_period" numeric DEFAULT 50000 NOT NULL,
    "max_period_total" numeric DEFAULT 100000 NOT NULL,
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: lottery_play_limits_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."lottery_play_limits_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: lottery_play_limits_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."lottery_play_limits_id_seq" OWNED BY "public"."lottery_play_limits"."id";


--
-- Name: member_chat_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."member_chat_messages" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "username" character varying(50) NOT NULL,
    "nickname" character varying(80) NOT NULL,
    "room_type" character varying(20) NOT NULL,
    "scope" character varying(64) DEFAULT 'lobby'::character varying NOT NULL,
    "room_scope" character varying(64) DEFAULT 'legacy'::character varying NOT NULL,
    "game_id" character varying(40) DEFAULT 'legacy'::character varying NOT NULL,
    "content" character varying(500) NOT NULL,
    "message_type" character varying(20) DEFAULT 'text'::character varying NOT NULL,
    "reference_id" bigint DEFAULT 0 NOT NULL,
    "red_packet_count" bigint DEFAULT 0 NOT NULL,
    "red_packet_total_cents" bigint DEFAULT 0 NOT NULL,
    "red_packet_min_turnover_cents" bigint DEFAULT 0 NOT NULL,
    "red_packet_cover" character varying(24) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "deleted_by" character varying(80),
    "cleanup_request_id" character varying(96) DEFAULT ''::character varying NOT NULL
);


--
-- Name: member_chat_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."member_chat_messages_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: member_chat_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."member_chat_messages_id_seq" OWNED BY "public"."member_chat_messages"."id";


--
-- Name: member_notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."member_notifications" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "game_id" character varying(40),
    "room_scope" character varying(64),
    "event_key" character varying(180),
    "title" character varying(120) NOT NULL,
    "content" "text",
    "level" character varying(20) DEFAULT 'info'::character varying NOT NULL,
    "category" character varying(30) DEFAULT 'system'::character varying NOT NULL,
    "link" character varying(300),
    "read" boolean DEFAULT false NOT NULL,
    "game_name" character varying(80),
    "issue" character varying(64),
    "draw_numbers" character varying(120),
    "draw_at" timestamp with time zone,
    "bet_count" bigint DEFAULT 0 NOT NULL,
    "won_count" bigint DEFAULT 0 NOT NULL,
    "stake_cents" bigint DEFAULT 0 NOT NULL,
    "payout_cents" bigint DEFAULT 0 NOT NULL,
    "bet_details_json" "text",
    "created_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    "deleted_by" character varying(80) DEFAULT ''::character varying NOT NULL,
    "cleanup_request_id" character varying(96) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT "chk_member_notification_financials" CHECK ((("bet_count" >= 0) AND ("won_count" >= 0) AND ("won_count" <= "bet_count") AND ("stake_cents" >= 0) AND ("payout_cents" >= 0) AND (("category")::"text" = ANY ((ARRAY['system'::character varying, 'account'::character varying, 'activity'::character varying, 'winning'::character varying])::"text"[])) AND (("level")::"text" = ANY ((ARRAY['info'::character varying, 'success'::character varying, 'warning'::character varying, 'error'::character varying])::"text"[]))))
);


--
-- Name: member_notifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."member_notifications_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: member_notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."member_notifications_id_seq" OWNED BY "public"."member_notifications"."id";


--
-- Name: member_payment_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."member_payment_accounts" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "account_type" character varying(30) NOT NULL,
    "label" character varying(80) NOT NULL,
    "account_name" character varying(100) NOT NULL,
    "account_no" "text" NOT NULL,
    "holder_name" character varying(80),
    "is_default" boolean DEFAULT false NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    CONSTRAINT "chk_member_payment_account_type" CHECK ((("account_type")::"text" = ANY ((ARRAY['bank'::character varying, 'alipay'::character varying, 'wechat'::character varying, 'usdt'::character varying])::"text"[])))
);


--
-- Name: member_payment_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."member_payment_accounts_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: member_payment_accounts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."member_payment_accounts_id_seq" OWNED BY "public"."member_payment_accounts"."id";


--
-- Name: member_public_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."member_public_id_seq"
    START WITH 1000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."ops_activities" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "type" character varying(30) NOT NULL,
    "title" character varying(120) NOT NULL,
    "subtitle" character varying(200),
    "status" character varying(20) DEFAULT 'draft'::character varying NOT NULL,
    "cover" character varying(320),
    "reward_cents" bigint DEFAULT 0 NOT NULL,
    "pool_total_cents" bigint DEFAULT 0 NOT NULL,
    "pool_remaining_cents" bigint DEFAULT 0 NOT NULL,
    "config_json" "text",
    "participants" bigint DEFAULT 0 NOT NULL,
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "starts_at" timestamp with time zone,
    "ends_at" timestamp with time zone,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    CONSTRAINT "chk_ops_activity_financials" CHECK ((("reward_cents" >= 0) AND ("pool_total_cents" >= 0) AND ("pool_remaining_cents" >= 0) AND ("pool_remaining_cents" <= "pool_total_cents") AND ("participants" >= 0)))
);


--
-- Name: ops_activities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."ops_activities_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ops_activities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."ops_activities_id_seq" OWNED BY "public"."ops_activities"."id";


--
-- Name: plan_recommendations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."plan_recommendations" (
    "id" bigint NOT NULL,
    "workspace_id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "issue" character varying(64) NOT NULL,
    "master_name" character varying(60) NOT NULL,
    "master_title" character varying(80) DEFAULT ''::character varying NOT NULL,
    "master_color" character varying(16) DEFAULT '#2aa9b3'::character varying NOT NULL,
    "numbers" character varying(120) DEFAULT ''::character varying NOT NULL,
    "size" character varying(4) DEFAULT ''::character varying NOT NULL,
    "parity" character varying(4) DEFAULT ''::character varying NOT NULL,
    "result" character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    "note" character varying(500) DEFAULT ''::character varying NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "sort_order" bigint DEFAULT 100 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone
);


--
-- Name: plan_recommendations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."plan_recommendations_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plan_recommendations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."plan_recommendations_id_seq" OWNED BY "public"."plan_recommendations"."id";


--
-- Name: rebate_daily_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."rebate_daily_records" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "biz_date" character varying(10) NOT NULL,
    "user_id" bigint NOT NULL,
    "username" character varying(50) NOT NULL,
    "turnover_cents" bigint DEFAULT 0 NOT NULL,
    "rate_percent" numeric DEFAULT 0 NOT NULL,
    "amount_cents" bigint DEFAULT 0 NOT NULL,
    "status" character varying(20) DEFAULT 'credited'::character varying NOT NULL,
    "operator" character varying(80),
    "created_at" timestamp with time zone,
    CONSTRAINT "chk_rebate_daily_financials" CHECK ((("turnover_cents" >= 0) AND (("rate_percent" >= (0)::numeric) AND ("rate_percent" <= (100)::numeric)) AND ("amount_cents" >= 0) AND (("status")::"text" = 'credited'::"text")))
);


--
-- Name: rebate_daily_records_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."rebate_daily_records_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: rebate_daily_records_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."rebate_daily_records_id_seq" OWNED BY "public"."rebate_daily_records"."id";


--
-- Name: room_game_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."room_game_settings" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "agent_id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "enabled" boolean NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: room_game_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."room_game_settings_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: room_game_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."room_game_settings_id_seq" OWNED BY "public"."room_game_settings"."id";


--
-- Name: room_play_odds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."room_play_odds" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "agent_id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "play_code" character varying(40) NOT NULL,
    "odds" numeric NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: room_play_odds_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."room_play_odds_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: room_play_odds_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."room_play_odds_id_seq" OWNED BY "public"."room_play_odds"."id";


--
-- Name: special_number_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."special_number_campaigns" (
    "id" bigint NOT NULL,
    "title" character varying(120) NOT NULL,
    "status" character varying(20) DEFAULT 'draft'::character varying NOT NULL,
    "rule_text" character varying(500),
    "granted_count" bigint DEFAULT 0 NOT NULL,
    "starts_at" timestamp with time zone,
    "ends_at" timestamp with time zone,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: special_number_campaigns_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."special_number_campaigns_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: special_number_campaigns_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."special_number_campaigns_id_seq" OWNED BY "public"."special_number_campaigns"."id";


--
-- Name: special_number_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."special_number_grants" (
    "id" bigint NOT NULL,
    "campaign_id" bigint NOT NULL,
    "resource_id" bigint NOT NULL,
    "number" character varying(40) NOT NULL,
    "user_id" bigint NOT NULL,
    "username" character varying(50) NOT NULL,
    "operator" character varying(80),
    "created_at" timestamp with time zone
);


--
-- Name: special_number_grants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."special_number_grants_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: special_number_grants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."special_number_grants_id_seq" OWNED BY "public"."special_number_grants"."id";


--
-- Name: special_number_resources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."special_number_resources" (
    "id" bigint NOT NULL,
    "number" character varying(40) NOT NULL,
    "level" character varying(20) DEFAULT 'normal'::character varying NOT NULL,
    "status" character varying(20) DEFAULT 'available'::character varying NOT NULL,
    "owner_user_id" bigint,
    "price_cents" bigint DEFAULT 0 NOT NULL,
    "remark" character varying(300),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: special_number_resources_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."special_number_resources_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: special_number_resources_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."special_number_resources_id_seq" OWNED BY "public"."special_number_resources"."id";


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."system_settings" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "room_name" character varying(80) DEFAULT '王者'::character varying NOT NULL,
    "room_logo" "text",
    "room_code" character varying(40) DEFAULT '1231'::character varying NOT NULL,
    "chat_nickname" character varying(80) DEFAULT '群主'::character varying NOT NULL,
    "nickname_display_length" bigint DEFAULT 0 NOT NULL,
    "min_chat_score" numeric DEFAULT 0 NOT NULL,
    "min_credit_amount" numeric DEFAULT 0 NOT NULL,
    "min_debit_amount" numeric DEFAULT 0 NOT NULL,
    "room_enabled" boolean DEFAULT true NOT NULL,
    "require_join_review" boolean DEFAULT true NOT NULL,
    "sound_enabled" boolean DEFAULT true NOT NULL,
    "show_odds" boolean DEFAULT true NOT NULL,
    "prediction_enabled" boolean DEFAULT true NOT NULL,
    "abnormal_login_alert" boolean DEFAULT false NOT NULL,
    "security_password_check" boolean DEFAULT false NOT NULL,
    "room_notice" character varying(2000),
    "announcements_json" "text",
    "game_settings_json" "text",
    "quick_replies_json" "text",
    "rebate_settings_json" "text",
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: system_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."system_settings_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."system_settings_id_seq" OWNED BY "public"."system_settings"."id";


--
-- Name: user; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."user" (
    "user_id" bigint NOT NULL,
    "public_id" bigint DEFAULT "nextval"('"public"."member_public_id_seq"'::"regclass") NOT NULL,
    "username" character varying(50) NOT NULL,
    "login_scope" character varying(80) DEFAULT 'platform'::character varying NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "email" character varying(100),
    "password" character varying(255) NOT NULL,
    "auth_version" bigint DEFAULT 1 NOT NULL,
    "nickname" character varying(50),
    "phone" character varying(30),
    "role" character varying(20) DEFAULT 'member'::character varying NOT NULL,
    "remark" character varying(500),
    "robot_game_ids_json" "text" DEFAULT '[]'::"text" NOT NULL,
    "robot_active_start" character varying(5) DEFAULT ''::character varying NOT NULL,
    "robot_active_end" character varying(5) DEFAULT ''::character varying NOT NULL,
    "robot_min_bet_cents" bigint DEFAULT 0 NOT NULL,
    "robot_max_bet_cents" bigint DEFAULT 0 NOT NULL,
    "risk_level" character varying(20) DEFAULT 'normal'::character varying NOT NULL,
    "balance_cents" bigint DEFAULT 0 NOT NULL,
    "fly_mode" character varying(20) DEFAULT 'inherit'::character varying NOT NULL,
    "fly_rate" numeric DEFAULT 0 NOT NULL,
    "room_rebate_rate" numeric DEFAULT 0 NOT NULL,
    "room_profit_share_rate" numeric DEFAULT 0 NOT NULL,
    "rebate_mode" character varying(20) DEFAULT 'inherit'::character varying NOT NULL,
    "rebate_rate" numeric DEFAULT 0 NOT NULL,
    "agent_room_code" character varying(40),
    "agent_room_name" character varying(50),
    "agent_room_logo" "text",
    "group_chat_enabled" boolean DEFAULT false NOT NULL,
    "parent_agent_id" bigint,
    "parent_tenant_id" bigint,
    "status" bigint DEFAULT 1,
    "muted_until" timestamp with time zone,
    "mute_reason" character varying(300),
    "last_login_at" timestamp with time zone,
    "login_count" bigint DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    CONSTRAINT "chk_user_balance_nonnegative" CHECK (("balance_cents" >= 0))
);


--
-- Name: user_applications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."user_applications" (
    "id" bigint NOT NULL,
    "request_id" character varying(96) DEFAULT ''::character varying NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "username" character varying(50) NOT NULL,
    "account_type" character varying(20) NOT NULL,
    "request_type" character varying(20) NOT NULL,
    "payment_type" character varying(30) NOT NULL,
    "payment_account_id" bigint DEFAULT 0 NOT NULL,
    "payment_account_label" character varying(180),
    "room_scope" character varying(64) DEFAULT ''::character varying NOT NULL,
    "target_room_code" character varying(40) DEFAULT ''::character varying NOT NULL,
    "game_id" character varying(40) DEFAULT ''::character varying NOT NULL,
    "chat_message_id" bigint DEFAULT 0 NOT NULL,
    "requested_cents" bigint DEFAULT 0 NOT NULL,
    "received_cents" bigint DEFAULT 0 NOT NULL,
    "remark" character varying(500),
    "status" character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "operator" character varying(80),
    "review_remark" character varying(500),
    "reviewed_at" timestamp with time zone,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    CONSTRAINT "chk_user_application_financials" CHECK (((((("request_type")::"text" = ANY ((ARRAY['credit'::character varying, 'debit'::character varying])::"text"[])) AND ("requested_cents" > 0)) OR ((("request_type")::"text" = ANY ((ARRAY['agent'::character varying, 'join'::character varying])::"text"[])) AND ("requested_cents" = 0))) AND ("received_cents" >= 0) AND (("status")::"text" = ANY ((ARRAY['pending'::character varying, 'approved'::character varying, 'rejected'::character varying])::"text"[]))))
);


--
-- Name: user_applications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."user_applications_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_applications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."user_applications_id_seq" OWNED BY "public"."user_applications"."id";


--
-- Name: user_balance_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."user_balance_transactions" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "reference" character varying(180) DEFAULT ''::character varying NOT NULL,
    "amount_cents" bigint NOT NULL,
    "before_cents" bigint NOT NULL,
    "after_cents" bigint NOT NULL,
    "type" character varying(30) DEFAULT 'manual'::character varying NOT NULL,
    "remark" character varying(300),
    "operator" character varying(80),
    "created_at" timestamp with time zone,
    CONSTRAINT "chk_balance_ledger_arithmetic" CHECK ((("after_cents" = ("before_cents" + "amount_cents")) AND ("before_cents" >= 0) AND ("after_cents" >= 0)))
);


--
-- Name: user_balance_transactions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."user_balance_transactions_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_balance_transactions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."user_balance_transactions_id_seq" OWNED BY "public"."user_balance_transactions"."id";


--
-- Name: user_play_odds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."user_play_odds" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "user_id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "play_code" character varying(40) NOT NULL,
    "odds" numeric NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: user_play_odds_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."user_play_odds_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_play_odds_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."user_play_odds_id_seq" OWNED BY "public"."user_play_odds"."id";


--
-- Name: user_user_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."user_user_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."user_user_id_seq" OWNED BY "public"."user"."user_id";


--
-- Name: wallet_payment_channels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."wallet_payment_channels" (
    "id" bigint NOT NULL,
    "workspace_id" bigint DEFAULT 0 NOT NULL,
    "provider" character varying(80) NOT NULL,
    "name" character varying(80) NOT NULL,
    "merchant_no" character varying(120),
    "credit_type" character varying(30) NOT NULL,
    "fee_rate" numeric DEFAULT 0 NOT NULL,
    "min_amount" numeric DEFAULT 0 NOT NULL,
    "max_amount" numeric DEFAULT 0 NOT NULL,
    "status" character varying(20) DEFAULT 'enabled'::character varying NOT NULL,
    "remark" character varying(500),
    "sort_order" bigint DEFAULT 0 NOT NULL,
    "mode" character varying(20) DEFAULT 'manual'::character varying NOT NULL,
    "api_base" character varying(500),
    "create_order_path" character varying(300),
    "query_order_path" character varying(300),
    "callback_path" character varying(300),
    "secret_key" "text",
    "timeout_seconds" bigint DEFAULT 10 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone,
    "deleted_at" timestamp with time zone,
    CONSTRAINT "chk_wallet_payment_channel_financials" CHECK (((("fee_rate" >= (0)::numeric) AND ("fee_rate" <= (100)::numeric)) AND ("min_amount" >= (0)::numeric) AND ("max_amount" >= (0)::numeric) AND (("max_amount" = (0)::numeric) OR ("max_amount" >= "min_amount")) AND (("credit_type")::"text" = ANY ((ARRAY['manual'::character varying, 'bank'::character varying, 'alipay'::character varying, 'wechat'::character varying, 'usdt'::character varying])::"text"[])) AND (("status")::"text" = ANY ((ARRAY['enabled'::character varying, 'disabled'::character varying])::"text"[]))))
);


--
-- Name: wallet_payment_channels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."wallet_payment_channels_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: wallet_payment_channels_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."wallet_payment_channels_id_seq" OWNED BY "public"."wallet_payment_channels"."id";


--
-- Name: workspace_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."workspace_memberships" (
    "id" bigint NOT NULL,
    "workspace_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "role" character varying(20) DEFAULT 'member'::character varying NOT NULL,
    "status" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: workspace_memberships_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."workspace_memberships_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: workspace_memberships_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."workspace_memberships_id_seq" OWNED BY "public"."workspace_memberships"."id";


--
-- Name: workspace_robot_games; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."workspace_robot_games" (
    "id" bigint NOT NULL,
    "robot_id" bigint NOT NULL,
    "game_id" character varying(40) NOT NULL,
    "created_at" timestamp with time zone
);


--
-- Name: workspace_robot_games_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."workspace_robot_games_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: workspace_robot_games_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."workspace_robot_games_id_seq" OWNED BY "public"."workspace_robot_games"."id";


--
-- Name: workspace_robot_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."workspace_robot_profiles" (
    "id" bigint NOT NULL,
    "workspace_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "avatar" character varying(300) DEFAULT ''::character varying NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "active_start" character varying(5) DEFAULT ''::character varying NOT NULL,
    "active_end" character varying(5) DEFAULT ''::character varying NOT NULL,
    "min_bet_cents" bigint DEFAULT 100 NOT NULL,
    "max_bet_cents" bigint DEFAULT 5000 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: workspace_robot_profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."workspace_robot_profiles_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: workspace_robot_profiles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."workspace_robot_profiles_id_seq" OWNED BY "public"."workspace_robot_profiles"."id";


--
-- Name: workspace_robot_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."workspace_robot_settings" (
    "workspace_id" bigint NOT NULL,
    "enabled" boolean DEFAULT false NOT NULL,
    "interval_secs" bigint DEFAULT 60 NOT NULL,
    "bets_per_cycle" bigint DEFAULT 1 NOT NULL,
    "daily_bet_limit" bigint DEFAULT 200 NOT NULL,
    "max_pending_bets" bigint DEFAULT 50 NOT NULL,
    "pause_reason" character varying(240) DEFAULT ''::character varying NOT NULL,
    "last_run_at" timestamp with time zone,
    "last_error" character varying(500),
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: workspace_robot_settings_workspace_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."workspace_robot_settings_workspace_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: workspace_robot_settings_workspace_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."workspace_robot_settings_workspace_id_seq" OWNED BY "public"."workspace_robot_settings"."workspace_id";


--
-- Name: workspaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."workspaces" (
    "id" bigint NOT NULL,
    "code" character varying(40) NOT NULL,
    "room_code" character varying(40) DEFAULT ''::character varying NOT NULL,
    "type" character varying(20) NOT NULL,
    "owner_user_id" bigint NOT NULL,
    "parent_id" bigint,
    "scope" character varying(64) NOT NULL,
    "name" character varying(80) NOT NULL,
    "logo" "text",
    "status" bigint DEFAULT 1 NOT NULL,
    "created_at" timestamp with time zone,
    "updated_at" timestamp with time zone
);


--
-- Name: workspaces_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."workspaces_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: workspaces_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."workspaces_id_seq" OWNED BY "public"."workspaces"."id";


--
-- Name: activity_participations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."activity_participations" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."activity_participations_id_seq"'::"regclass");


--
-- Name: admin_audit_logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."admin_audit_logs" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."admin_audit_logs_id_seq"'::"regclass");


--
-- Name: admin_notifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."admin_notifications" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."admin_notifications_id_seq"'::"regclass");


--
-- Name: agent_profit_share_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."agent_profit_share_records" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."agent_profit_share_records_id_seq"'::"regclass");


--
-- Name: chat_red_packet_claims id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."chat_red_packet_claims" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."chat_red_packet_claims_id_seq"'::"regclass");


--
-- Name: chat_red_packets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."chat_red_packets" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."chat_red_packets_id_seq"'::"regclass");


--
-- Name: entertainment_platforms id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."entertainment_platforms" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."entertainment_platforms_id_seq"'::"regclass");


--
-- Name: lottery_assistant_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_assistant_requests" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_assistant_requests_id_seq"'::"regclass");


--
-- Name: lottery_bet_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_bet_requests" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_bet_requests_id_seq"'::"regclass");


--
-- Name: lottery_bets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_bets" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_bets_id_seq"'::"regclass");


--
-- Name: lottery_draws id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_draws" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_draws_id_seq"'::"regclass");


--
-- Name: lottery_issues id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_issues" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_issues_id_seq"'::"regclass");


--
-- Name: lottery_lobby_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_lobby_categories" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_lobby_categories_id_seq"'::"regclass");


--
-- Name: lottery_play_limits id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_play_limits" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."lottery_play_limits_id_seq"'::"regclass");


--
-- Name: member_chat_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_chat_messages" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."member_chat_messages_id_seq"'::"regclass");


--
-- Name: member_notifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_notifications" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."member_notifications_id_seq"'::"regclass");


--
-- Name: member_payment_accounts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_payment_accounts" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."member_payment_accounts_id_seq"'::"regclass");


--
-- Name: ops_activities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."ops_activities" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."ops_activities_id_seq"'::"regclass");


--
-- Name: plan_recommendations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."plan_recommendations" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."plan_recommendations_id_seq"'::"regclass");


--
-- Name: rebate_daily_records id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."rebate_daily_records" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."rebate_daily_records_id_seq"'::"regclass");


--
-- Name: room_game_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."room_game_settings" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."room_game_settings_id_seq"'::"regclass");


--
-- Name: room_play_odds id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."room_play_odds" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."room_play_odds_id_seq"'::"regclass");


--
-- Name: special_number_campaigns id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_campaigns" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."special_number_campaigns_id_seq"'::"regclass");


--
-- Name: special_number_grants id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_grants" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."special_number_grants_id_seq"'::"regclass");


--
-- Name: special_number_resources id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_resources" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."special_number_resources_id_seq"'::"regclass");


--
-- Name: system_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."system_settings" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."system_settings_id_seq"'::"regclass");


--
-- Name: user user_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user" ALTER COLUMN "user_id" SET DEFAULT "nextval"('"public"."user_user_id_seq"'::"regclass");


--
-- Name: user_applications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_applications" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."user_applications_id_seq"'::"regclass");


--
-- Name: user_balance_transactions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_balance_transactions" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."user_balance_transactions_id_seq"'::"regclass");


--
-- Name: user_play_odds id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_play_odds" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."user_play_odds_id_seq"'::"regclass");


--
-- Name: wallet_payment_channels id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."wallet_payment_channels" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."wallet_payment_channels_id_seq"'::"regclass");


--
-- Name: workspace_memberships id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_memberships" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."workspace_memberships_id_seq"'::"regclass");


--
-- Name: workspace_robot_games id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_games" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."workspace_robot_games_id_seq"'::"regclass");


--
-- Name: workspace_robot_profiles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_profiles" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."workspace_robot_profiles_id_seq"'::"regclass");


--
-- Name: workspace_robot_settings workspace_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_settings" ALTER COLUMN "workspace_id" SET DEFAULT "nextval"('"public"."workspace_robot_settings_workspace_id_seq"'::"regclass");


--
-- Name: workspaces id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspaces" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."workspaces_id_seq"'::"regclass");


--
-- Name: activity_participations activity_participations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."activity_participations"
    ADD CONSTRAINT "activity_participations_pkey" PRIMARY KEY ("id");


--
-- Name: admin_audit_logs admin_audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."admin_audit_logs"
    ADD CONSTRAINT "admin_audit_logs_pkey" PRIMARY KEY ("id");


--
-- Name: admin_notifications admin_notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."admin_notifications"
    ADD CONSTRAINT "admin_notifications_pkey" PRIMARY KEY ("id");


--
-- Name: agent_profit_share_records agent_profit_share_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."agent_profit_share_records"
    ADD CONSTRAINT "agent_profit_share_records_pkey" PRIMARY KEY ("id");


--
-- Name: chat_red_packet_claims chat_red_packet_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."chat_red_packet_claims"
    ADD CONSTRAINT "chat_red_packet_claims_pkey" PRIMARY KEY ("id");


--
-- Name: chat_red_packets chat_red_packets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."chat_red_packets"
    ADD CONSTRAINT "chat_red_packets_pkey" PRIMARY KEY ("id");


--
-- Name: entertainment_platforms entertainment_platforms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."entertainment_platforms"
    ADD CONSTRAINT "entertainment_platforms_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_assistant_requests lottery_assistant_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_assistant_requests"
    ADD CONSTRAINT "lottery_assistant_requests_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_bet_requests lottery_bet_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_bet_requests"
    ADD CONSTRAINT "lottery_bet_requests_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_bets lottery_bets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_bets"
    ADD CONSTRAINT "lottery_bets_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_draws lottery_draws_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_draws"
    ADD CONSTRAINT "lottery_draws_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_games lottery_games_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_games"
    ADD CONSTRAINT "lottery_games_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_issues lottery_issues_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_issues"
    ADD CONSTRAINT "lottery_issues_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_lobby_categories lottery_lobby_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_lobby_categories"
    ADD CONSTRAINT "lottery_lobby_categories_pkey" PRIMARY KEY ("id");


--
-- Name: lottery_play_limits lottery_play_limits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_play_limits"
    ADD CONSTRAINT "lottery_play_limits_pkey" PRIMARY KEY ("id");


--
-- Name: member_chat_messages member_chat_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_chat_messages"
    ADD CONSTRAINT "member_chat_messages_pkey" PRIMARY KEY ("id");


--
-- Name: member_notifications member_notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_notifications"
    ADD CONSTRAINT "member_notifications_pkey" PRIMARY KEY ("id");


--
-- Name: member_payment_accounts member_payment_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_payment_accounts"
    ADD CONSTRAINT "member_payment_accounts_pkey" PRIMARY KEY ("id");


--
-- Name: ops_activities ops_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."ops_activities"
    ADD CONSTRAINT "ops_activities_pkey" PRIMARY KEY ("id");


--
-- Name: plan_recommendations plan_recommendations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."plan_recommendations"
    ADD CONSTRAINT "plan_recommendations_pkey" PRIMARY KEY ("id");


--
-- Name: rebate_daily_records rebate_daily_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."rebate_daily_records"
    ADD CONSTRAINT "rebate_daily_records_pkey" PRIMARY KEY ("id");


--
-- Name: room_game_settings room_game_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."room_game_settings"
    ADD CONSTRAINT "room_game_settings_pkey" PRIMARY KEY ("id");


--
-- Name: room_play_odds room_play_odds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."room_play_odds"
    ADD CONSTRAINT "room_play_odds_pkey" PRIMARY KEY ("id");


--
-- Name: special_number_campaigns special_number_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_campaigns"
    ADD CONSTRAINT "special_number_campaigns_pkey" PRIMARY KEY ("id");


--
-- Name: special_number_grants special_number_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_grants"
    ADD CONSTRAINT "special_number_grants_pkey" PRIMARY KEY ("id");


--
-- Name: special_number_resources special_number_resources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."special_number_resources"
    ADD CONSTRAINT "special_number_resources_pkey" PRIMARY KEY ("id");


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."system_settings"
    ADD CONSTRAINT "system_settings_pkey" PRIMARY KEY ("id");


--
-- Name: user_applications user_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_applications"
    ADD CONSTRAINT "user_applications_pkey" PRIMARY KEY ("id");


--
-- Name: user_balance_transactions user_balance_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_balance_transactions"
    ADD CONSTRAINT "user_balance_transactions_pkey" PRIMARY KEY ("id");


--
-- Name: user user_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user"
    ADD CONSTRAINT "user_pkey" PRIMARY KEY ("user_id");


--
-- Name: user_play_odds user_play_odds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_play_odds"
    ADD CONSTRAINT "user_play_odds_pkey" PRIMARY KEY ("id");


--
-- Name: wallet_payment_channels wallet_payment_channels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."wallet_payment_channels"
    ADD CONSTRAINT "wallet_payment_channels_pkey" PRIMARY KEY ("id");


--
-- Name: workspace_memberships workspace_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_memberships"
    ADD CONSTRAINT "workspace_memberships_pkey" PRIMARY KEY ("id");


--
-- Name: workspace_robot_games workspace_robot_games_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_games"
    ADD CONSTRAINT "workspace_robot_games_pkey" PRIMARY KEY ("id");


--
-- Name: workspace_robot_profiles workspace_robot_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_profiles"
    ADD CONSTRAINT "workspace_robot_profiles_pkey" PRIMARY KEY ("id");


--
-- Name: workspace_robot_settings workspace_robot_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspace_robot_settings"
    ADD CONSTRAINT "workspace_robot_settings_pkey" PRIMARY KEY ("workspace_id");


--
-- Name: workspaces workspaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."workspaces"
    ADD CONSTRAINT "workspaces_pkey" PRIMARY KEY ("id");


--
-- Name: idx_activity_participations_participated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_activity_participations_participated_at" ON "public"."activity_participations" USING "btree" ("participated_at");


--
-- Name: idx_activity_participations_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_activity_participations_workspace_id" ON "public"."activity_participations" USING "btree" ("workspace_id");


--
-- Name: idx_admin_audit_logs_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_actor_id" ON "public"."admin_audit_logs" USING "btree" ("actor_id");


--
-- Name: idx_admin_audit_logs_actor_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_actor_role" ON "public"."admin_audit_logs" USING "btree" ("actor_role");


--
-- Name: idx_admin_audit_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_created_at" ON "public"."admin_audit_logs" USING "btree" ("created_at");


--
-- Name: idx_admin_audit_logs_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_event_id" ON "public"."admin_audit_logs" USING "btree" ("event_id");


--
-- Name: idx_admin_audit_logs_path; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_path" ON "public"."admin_audit_logs" USING "btree" ("path");


--
-- Name: idx_admin_audit_logs_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_request_id" ON "public"."admin_audit_logs" USING "btree" ("request_id");


--
-- Name: idx_admin_audit_logs_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_room_scope" ON "public"."admin_audit_logs" USING "btree" ("room_scope");


--
-- Name: idx_admin_audit_logs_target_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_target_ref" ON "public"."admin_audit_logs" USING "btree" ("target_ref");


--
-- Name: idx_admin_audit_logs_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_audit_logs_workspace_id" ON "public"."admin_audit_logs" USING "btree" ("workspace_id");


--
-- Name: idx_admin_notifications_cleanup_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_notifications_cleanup_request_id" ON "public"."admin_notifications" USING "btree" ("cleanup_request_id");


--
-- Name: idx_admin_notifications_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_notifications_created_at" ON "public"."admin_notifications" USING "btree" ("created_at");


--
-- Name: idx_admin_notifications_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_notifications_deleted_at" ON "public"."admin_notifications" USING "btree" ("deleted_at");


--
-- Name: idx_admin_notifications_read; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_notifications_read" ON "public"."admin_notifications" USING "btree" ("read");


--
-- Name: idx_admin_notifications_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_admin_notifications_workspace_id" ON "public"."admin_notifications" USING "btree" ("workspace_id");


--
-- Name: idx_agent_profit_share_records_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_agent_id" ON "public"."agent_profit_share_records" USING "btree" ("agent_id");


--
-- Name: idx_agent_profit_share_records_agent_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_agent_username" ON "public"."agent_profit_share_records" USING "btree" ("agent_username");


--
-- Name: idx_agent_profit_share_records_last_transaction_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_last_transaction_id" ON "public"."agent_profit_share_records" USING "btree" ("last_transaction_id");


--
-- Name: idx_agent_profit_share_records_room_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_room_code" ON "public"."agent_profit_share_records" USING "btree" ("room_code");


--
-- Name: idx_agent_profit_share_records_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_room_scope" ON "public"."agent_profit_share_records" USING "btree" ("room_scope");


--
-- Name: idx_agent_profit_share_records_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_status" ON "public"."agent_profit_share_records" USING "btree" ("status");


--
-- Name: idx_agent_profit_share_records_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_agent_profit_share_records_workspace_id" ON "public"."agent_profit_share_records" USING "btree" ("workspace_id");


--
-- Name: idx_assistant_request; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_assistant_request" ON "public"."lottery_assistant_requests" USING "btree" ("user_id", "request_id");


--
-- Name: idx_balance_ledger_user_id_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_balance_ledger_user_id_id" ON "public"."user_balance_transactions" USING "btree" ("user_id", "id");


--
-- Name: idx_balance_ledger_user_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_balance_ledger_user_reference" ON "public"."user_balance_transactions" USING "btree" ("user_id", "reference") WHERE (("reference")::"text" <> ''::"text");


--
-- Name: idx_bet_dedupe; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_bet_dedupe" ON "public"."lottery_bets" USING "btree" ("game_id", "issue", "room_scope", "user_id", "play_code", "position", "selection");


--
-- Name: idx_bet_feed_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_bet_feed_scope" ON "public"."lottery_bets" USING "btree" ("room_scope", "game_id", "issue", "created_at");


--
-- Name: idx_bet_request; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_bet_request" ON "public"."lottery_bet_requests" USING "btree" ("user_id", "request_id");


--
-- Name: idx_bet_workspace_feed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_bet_workspace_feed" ON "public"."lottery_bets" USING "btree" ("workspace_id");


--
-- Name: idx_chat_packet_member; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_chat_packet_member" ON "public"."chat_red_packet_claims" USING "btree" ("packet_id", "user_id");


--
-- Name: idx_chat_red_packet_claims_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packet_claims_created_at" ON "public"."chat_red_packet_claims" USING "btree" ("created_at");


--
-- Name: idx_chat_red_packet_claims_packet_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packet_claims_packet_id" ON "public"."chat_red_packet_claims" USING "btree" ("packet_id");


--
-- Name: idx_chat_red_packet_claims_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packet_claims_user_id" ON "public"."chat_red_packet_claims" USING "btree" ("user_id");


--
-- Name: idx_chat_red_packet_claims_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packet_claims_workspace_id" ON "public"."chat_red_packet_claims" USING "btree" ("workspace_id");


--
-- Name: idx_chat_red_packets_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_created_at" ON "public"."chat_red_packets" USING "btree" ("created_at");


--
-- Name: idx_chat_red_packets_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_expires_at" ON "public"."chat_red_packets" USING "btree" ("expires_at");


--
-- Name: idx_chat_red_packets_funding_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_funding_user_id" ON "public"."chat_red_packets" USING "btree" ("funding_user_id");


--
-- Name: idx_chat_red_packets_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_game_id" ON "public"."chat_red_packets" USING "btree" ("game_id");


--
-- Name: idx_chat_red_packets_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_chat_red_packets_message_id" ON "public"."chat_red_packets" USING "btree" ("message_id");


--
-- Name: idx_chat_red_packets_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_room_scope" ON "public"."chat_red_packets" USING "btree" ("room_scope");


--
-- Name: idx_chat_red_packets_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_scope" ON "public"."chat_red_packets" USING "btree" ("scope");


--
-- Name: idx_chat_red_packets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_status" ON "public"."chat_red_packets" USING "btree" ("status");


--
-- Name: idx_chat_red_packets_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_red_packets_workspace_id" ON "public"."chat_red_packets" USING "btree" ("workspace_id");


--
-- Name: idx_chat_room_game_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_room_game_created" ON "public"."member_chat_messages" USING "btree" ("room_scope", "game_id", "created_at");


--
-- Name: idx_chat_workspace_game_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_chat_workspace_game_created" ON "public"."member_chat_messages" USING "btree" ("workspace_id", "game_id", "created_at");


--
-- Name: idx_entertainment_platforms_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_entertainment_platforms_code" ON "public"."entertainment_platforms" USING "btree" ("code");


--
-- Name: idx_entertainment_platforms_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_entertainment_platforms_status" ON "public"."entertainment_platforms" USING "btree" ("status");


--
-- Name: idx_lottery_assistant_requests_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_assistant_requests_created_at" ON "public"."lottery_assistant_requests" USING "btree" ("created_at");


--
-- Name: idx_lottery_assistant_requests_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_assistant_requests_workspace_id" ON "public"."lottery_assistant_requests" USING "btree" ("workspace_id");


--
-- Name: idx_lottery_bet_requests_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bet_requests_created_at" ON "public"."lottery_bet_requests" USING "btree" ("created_at");


--
-- Name: idx_lottery_bet_requests_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bet_requests_status" ON "public"."lottery_bet_requests" USING "btree" ("status");


--
-- Name: idx_lottery_bet_requests_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bet_requests_workspace_id" ON "public"."lottery_bet_requests" USING "btree" ("workspace_id");


--
-- Name: idx_lottery_bets_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_created_at" ON "public"."lottery_bets" USING "btree" ("created_at");


--
-- Name: idx_lottery_bets_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_game_id" ON "public"."lottery_bets" USING "btree" ("game_id");


--
-- Name: idx_lottery_bets_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_issue" ON "public"."lottery_bets" USING "btree" ("issue");


--
-- Name: idx_lottery_bets_reconciliation_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_reconciliation_status" ON "public"."lottery_bets" USING "btree" ("reconciliation_status");


--
-- Name: idx_lottery_bets_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_room_scope" ON "public"."lottery_bets" USING "btree" ("room_scope");


--
-- Name: idx_lottery_bets_settled_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_settled_at" ON "public"."lottery_bets" USING "btree" ("settled_at");


--
-- Name: idx_lottery_bets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_status" ON "public"."lottery_bets" USING "btree" ("status");


--
-- Name: idx_lottery_bets_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_user_id" ON "public"."lottery_bets" USING "btree" ("user_id");


--
-- Name: idx_lottery_bets_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_username" ON "public"."lottery_bets" USING "btree" ("username");


--
-- Name: idx_lottery_bets_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_bets_workspace_id" ON "public"."lottery_bets" USING "btree" ("workspace_id");


--
-- Name: idx_lottery_draw_game_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_lottery_draw_game_issue" ON "public"."lottery_draws" USING "btree" ("game_id", "issue");


--
-- Name: idx_lottery_draws_draw_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_draws_draw_at" ON "public"."lottery_draws" USING "btree" ("draw_at");


--
-- Name: idx_lottery_draws_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_draws_game_id" ON "public"."lottery_draws" USING "btree" ("game_id");


--
-- Name: idx_lottery_draws_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_draws_issue" ON "public"."lottery_draws" USING "btree" ("issue");


--
-- Name: idx_lottery_games_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_lottery_games_code" ON "public"."lottery_games" USING "btree" ("code");


--
-- Name: idx_lottery_games_lobby_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_games_lobby_category" ON "public"."lottery_games" USING "btree" ("lobby_category");


--
-- Name: idx_lottery_issue_game_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_lottery_issue_game_issue" ON "public"."lottery_issues" USING "btree" ("game_id", "issue");


--
-- Name: idx_lottery_issues_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_issues_game_id" ON "public"."lottery_issues" USING "btree" ("game_id");


--
-- Name: idx_lottery_issues_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_issues_issue" ON "public"."lottery_issues" USING "btree" ("issue");


--
-- Name: idx_lottery_issues_seal_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_issues_seal_at" ON "public"."lottery_issues" USING "btree" ("seal_at");


--
-- Name: idx_lottery_issues_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_issues_status" ON "public"."lottery_issues" USING "btree" ("status");


--
-- Name: idx_lottery_lobby_categories_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_lottery_lobby_categories_deleted_at" ON "public"."lottery_lobby_categories" USING "btree" ("deleted_at");


--
-- Name: idx_lottery_lobby_categories_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_lottery_lobby_categories_name" ON "public"."lottery_lobby_categories" USING "btree" ("name");


--
-- Name: idx_member_chat_messages_cleanup_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_cleanup_request_id" ON "public"."member_chat_messages" USING "btree" ("cleanup_request_id");


--
-- Name: idx_member_chat_messages_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_created_at" ON "public"."member_chat_messages" USING "btree" ("created_at");


--
-- Name: idx_member_chat_messages_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_deleted_at" ON "public"."member_chat_messages" USING "btree" ("deleted_at");


--
-- Name: idx_member_chat_messages_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_game_id" ON "public"."member_chat_messages" USING "btree" ("game_id");


--
-- Name: idx_member_chat_messages_message_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_message_type" ON "public"."member_chat_messages" USING "btree" ("message_type");


--
-- Name: idx_member_chat_messages_reference_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_reference_id" ON "public"."member_chat_messages" USING "btree" ("reference_id");


--
-- Name: idx_member_chat_messages_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_room_scope" ON "public"."member_chat_messages" USING "btree" ("room_scope");


--
-- Name: idx_member_chat_messages_room_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_room_type" ON "public"."member_chat_messages" USING "btree" ("room_type");


--
-- Name: idx_member_chat_messages_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_scope" ON "public"."member_chat_messages" USING "btree" ("scope");


--
-- Name: idx_member_chat_messages_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_user_id" ON "public"."member_chat_messages" USING "btree" ("user_id");


--
-- Name: idx_member_chat_messages_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_chat_messages_workspace_id" ON "public"."member_chat_messages" USING "btree" ("workspace_id");


--
-- Name: idx_member_notification_event_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_member_notification_event_key" ON "public"."member_notifications" USING "btree" ("event_key") WHERE (("event_key")::"text" <> ''::"text");


--
-- Name: idx_member_notifications_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_category" ON "public"."member_notifications" USING "btree" ("category");


--
-- Name: idx_member_notifications_cleanup_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_cleanup_request_id" ON "public"."member_notifications" USING "btree" ("cleanup_request_id");


--
-- Name: idx_member_notifications_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_created_at" ON "public"."member_notifications" USING "btree" ("created_at");


--
-- Name: idx_member_notifications_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_deleted_at" ON "public"."member_notifications" USING "btree" ("deleted_at");


--
-- Name: idx_member_notifications_event_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_event_key" ON "public"."member_notifications" USING "btree" ("event_key");


--
-- Name: idx_member_notifications_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_game_id" ON "public"."member_notifications" USING "btree" ("game_id");


--
-- Name: idx_member_notifications_read; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_read" ON "public"."member_notifications" USING "btree" ("read");


--
-- Name: idx_member_notifications_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_room_scope" ON "public"."member_notifications" USING "btree" ("room_scope");


--
-- Name: idx_member_notifications_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_user_id" ON "public"."member_notifications" USING "btree" ("user_id");


--
-- Name: idx_member_notifications_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_notifications_workspace_id" ON "public"."member_notifications" USING "btree" ("workspace_id");


--
-- Name: idx_member_payment_account_one_default; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_member_payment_account_one_default" ON "public"."member_payment_accounts" USING "btree" ("user_id") WHERE ("is_default" AND ("deleted_at" IS NULL));


--
-- Name: idx_member_payment_accounts_account_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_payment_accounts_account_type" ON "public"."member_payment_accounts" USING "btree" ("account_type");


--
-- Name: idx_member_payment_accounts_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_payment_accounts_deleted_at" ON "public"."member_payment_accounts" USING "btree" ("deleted_at");


--
-- Name: idx_member_payment_accounts_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_payment_accounts_user_id" ON "public"."member_payment_accounts" USING "btree" ("user_id");


--
-- Name: idx_member_payment_accounts_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_member_payment_accounts_workspace_id" ON "public"."member_payment_accounts" USING "btree" ("workspace_id");


--
-- Name: idx_odds_game_play; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_odds_game_play" ON "public"."lottery_play_limits" USING "btree" ("game_id", "play_code");


--
-- Name: idx_ops_activities_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_ops_activities_deleted_at" ON "public"."ops_activities" USING "btree" ("deleted_at");


--
-- Name: idx_ops_activities_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_ops_activities_status" ON "public"."ops_activities" USING "btree" ("status");


--
-- Name: idx_ops_activities_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_ops_activities_type" ON "public"."ops_activities" USING "btree" ("type");


--
-- Name: idx_ops_activities_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_ops_activities_workspace_id" ON "public"."ops_activities" USING "btree" ("workspace_id");


--
-- Name: idx_participation_daily_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_participation_daily_unique" ON "public"."activity_participations" USING "btree" ("workspace_id", "user_id", "activity_id", "action", "biz_date", "reference");


--
-- Name: idx_plan_recommendations_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_deleted_at" ON "public"."plan_recommendations" USING "btree" ("deleted_at");


--
-- Name: idx_plan_recommendations_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_enabled" ON "public"."plan_recommendations" USING "btree" ("enabled");


--
-- Name: idx_plan_recommendations_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_game_id" ON "public"."plan_recommendations" USING "btree" ("game_id");


--
-- Name: idx_plan_recommendations_issue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_issue" ON "public"."plan_recommendations" USING "btree" ("issue");


--
-- Name: idx_plan_recommendations_result; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_result" ON "public"."plan_recommendations" USING "btree" ("result");


--
-- Name: idx_plan_recommendations_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_plan_recommendations_workspace_id" ON "public"."plan_recommendations" USING "btree" ("workspace_id");


--
-- Name: idx_profit_share_agent_day; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_profit_share_agent_day" ON "public"."agent_profit_share_records" USING "btree" ("workspace_id", "biz_date", "agent_id");


--
-- Name: idx_rebate_daily_records_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_rebate_daily_records_username" ON "public"."rebate_daily_records" USING "btree" ("username");


--
-- Name: idx_rebate_daily_records_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_rebate_daily_records_workspace_id" ON "public"."rebate_daily_records" USING "btree" ("workspace_id");


--
-- Name: idx_rebate_user_day; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_rebate_user_day" ON "public"."rebate_daily_records" USING "btree" ("workspace_id", "biz_date", "user_id");


--
-- Name: idx_robot_game; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_robot_game" ON "public"."workspace_robot_games" USING "btree" ("robot_id", "game_id");


--
-- Name: idx_room_game_setting; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_room_game_setting" ON "public"."room_game_settings" USING "btree" ("agent_id", "game_id");


--
-- Name: idx_room_game_settings_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_room_game_settings_workspace_id" ON "public"."room_game_settings" USING "btree" ("workspace_id");


--
-- Name: idx_room_odds_game_play; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_room_odds_game_play" ON "public"."room_play_odds" USING "btree" ("workspace_id", "game_id", "play_code");


--
-- Name: idx_room_play_odds_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_room_play_odds_agent_id" ON "public"."room_play_odds" USING "btree" ("agent_id");


--
-- Name: idx_room_play_odds_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_room_play_odds_workspace_id" ON "public"."room_play_odds" USING "btree" ("workspace_id");


--
-- Name: idx_special_number_campaigns_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_campaigns_status" ON "public"."special_number_campaigns" USING "btree" ("status");


--
-- Name: idx_special_number_grants_campaign_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_grants_campaign_id" ON "public"."special_number_grants" USING "btree" ("campaign_id");


--
-- Name: idx_special_number_grants_resource_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_grants_resource_id" ON "public"."special_number_grants" USING "btree" ("resource_id");


--
-- Name: idx_special_number_grants_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_grants_user_id" ON "public"."special_number_grants" USING "btree" ("user_id");


--
-- Name: idx_special_number_resources_number; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_special_number_resources_number" ON "public"."special_number_resources" USING "btree" ("number");


--
-- Name: idx_special_number_resources_owner_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_resources_owner_user_id" ON "public"."special_number_resources" USING "btree" ("owner_user_id");


--
-- Name: idx_special_number_resources_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_special_number_resources_status" ON "public"."special_number_resources" USING "btree" ("status");


--
-- Name: idx_system_settings_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_system_settings_workspace_id" ON "public"."system_settings" USING "btree" ("workspace_id");


--
-- Name: idx_user_agent_room_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_user_agent_room_code" ON "public"."user" USING "btree" ("agent_room_code") WHERE ((("agent_room_code")::"text" <> ''::"text") AND ("deleted_at" IS NULL));


--
-- Name: idx_user_applications_chat_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_chat_message_id" ON "public"."user_applications" USING "btree" ("chat_message_id");


--
-- Name: idx_user_applications_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_created_at" ON "public"."user_applications" USING "btree" ("created_at");


--
-- Name: idx_user_applications_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_game_id" ON "public"."user_applications" USING "btree" ("game_id");


--
-- Name: idx_user_applications_payment_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_payment_account_id" ON "public"."user_applications" USING "btree" ("payment_account_id");


--
-- Name: idx_user_applications_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_request_id" ON "public"."user_applications" USING "btree" ("request_id");


--
-- Name: idx_user_applications_request_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_request_type" ON "public"."user_applications" USING "btree" ("request_type");


--
-- Name: idx_user_applications_room_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_room_scope" ON "public"."user_applications" USING "btree" ("room_scope");


--
-- Name: idx_user_applications_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_status" ON "public"."user_applications" USING "btree" ("status");


--
-- Name: idx_user_applications_target_room_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_target_room_code" ON "public"."user_applications" USING "btree" ("target_room_code");


--
-- Name: idx_user_applications_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_user_id" ON "public"."user_applications" USING "btree" ("user_id");


--
-- Name: idx_user_applications_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_username" ON "public"."user_applications" USING "btree" ("username");


--
-- Name: idx_user_applications_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_applications_workspace_id" ON "public"."user_applications" USING "btree" ("workspace_id");


--
-- Name: idx_user_balance_transactions_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_balance_transactions_created_at" ON "public"."user_balance_transactions" USING "btree" ("created_at");


--
-- Name: idx_user_balance_transactions_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_balance_transactions_reference" ON "public"."user_balance_transactions" USING "btree" ("reference");


--
-- Name: idx_user_balance_transactions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_balance_transactions_user_id" ON "public"."user_balance_transactions" USING "btree" ("user_id");


--
-- Name: idx_user_balance_transactions_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_balance_transactions_workspace_id" ON "public"."user_balance_transactions" USING "btree" ("workspace_id");


--
-- Name: idx_user_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_deleted_at" ON "public"."user" USING "btree" ("deleted_at");


--
-- Name: idx_user_login_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_user_login_identity" ON "public"."user" USING "btree" ("login_scope", "lower"(("username")::"text")) WHERE ("deleted_at" IS NULL);


--
-- Name: idx_user_login_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_login_scope" ON "public"."user" USING "btree" ("login_scope");


--
-- Name: idx_user_muted_until; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_muted_until" ON "public"."user" USING "btree" ("muted_until");


--
-- Name: idx_user_odds_game_play; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_user_odds_game_play" ON "public"."user_play_odds" USING "btree" ("workspace_id", "user_id", "game_id", "play_code");


--
-- Name: idx_user_parent_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_parent_agent_id" ON "public"."user" USING "btree" ("parent_agent_id");


--
-- Name: idx_user_parent_tenant_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_parent_tenant_id" ON "public"."user" USING "btree" ("parent_tenant_id");


--
-- Name: idx_user_phone; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_phone" ON "public"."user" USING "btree" ("phone");


--
-- Name: idx_user_play_odds_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_play_odds_workspace_id" ON "public"."user_play_odds" USING "btree" ("workspace_id");


--
-- Name: idx_user_public_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_user_public_id" ON "public"."user" USING "btree" ("public_id");


--
-- Name: idx_user_risk_level; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_risk_level" ON "public"."user" USING "btree" ("risk_level");


--
-- Name: idx_user_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_role" ON "public"."user" USING "btree" ("role");


--
-- Name: idx_user_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_username" ON "public"."user" USING "btree" ("username");


--
-- Name: idx_user_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_user_workspace_id" ON "public"."user" USING "btree" ("workspace_id");


--
-- Name: idx_wallet_payment_channels_credit_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_wallet_payment_channels_credit_type" ON "public"."wallet_payment_channels" USING "btree" ("credit_type");


--
-- Name: idx_wallet_payment_channels_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_wallet_payment_channels_deleted_at" ON "public"."wallet_payment_channels" USING "btree" ("deleted_at");


--
-- Name: idx_wallet_payment_channels_mode; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_wallet_payment_channels_mode" ON "public"."wallet_payment_channels" USING "btree" ("mode");


--
-- Name: idx_wallet_payment_channels_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_wallet_payment_channels_status" ON "public"."wallet_payment_channels" USING "btree" ("status");


--
-- Name: idx_wallet_payment_channels_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_wallet_payment_channels_workspace_id" ON "public"."wallet_payment_channels" USING "btree" ("workspace_id");


--
-- Name: idx_workspace_game_setting; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspace_game_setting" ON "public"."room_game_settings" USING "btree" ("workspace_id", "game_id");


--
-- Name: idx_workspace_member; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspace_member" ON "public"."workspace_memberships" USING "btree" ("workspace_id", "user_id");


--
-- Name: idx_workspace_memberships_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_memberships_status" ON "public"."workspace_memberships" USING "btree" ("status");


--
-- Name: idx_workspace_memberships_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_memberships_user_id" ON "public"."workspace_memberships" USING "btree" ("user_id");


--
-- Name: idx_workspace_memberships_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_memberships_workspace_id" ON "public"."workspace_memberships" USING "btree" ("workspace_id");


--
-- Name: idx_workspace_robot_games_game_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_robot_games_game_id" ON "public"."workspace_robot_games" USING "btree" ("game_id");


--
-- Name: idx_workspace_robot_games_robot_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_robot_games_robot_id" ON "public"."workspace_robot_games" USING "btree" ("robot_id");


--
-- Name: idx_workspace_robot_profiles_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_robot_profiles_enabled" ON "public"."workspace_robot_profiles" USING "btree" ("enabled");


--
-- Name: idx_workspace_robot_profiles_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspace_robot_profiles_user_id" ON "public"."workspace_robot_profiles" USING "btree" ("user_id");


--
-- Name: idx_workspace_robot_profiles_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_robot_profiles_workspace_id" ON "public"."workspace_robot_profiles" USING "btree" ("workspace_id");


--
-- Name: idx_workspace_robot_settings_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspace_robot_settings_enabled" ON "public"."workspace_robot_settings" USING "btree" ("enabled");


--
-- Name: idx_workspace_robot_user; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspace_robot_user" ON "public"."workspace_robot_profiles" USING "btree" ("workspace_id", "user_id");


--
-- Name: idx_workspaces_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspaces_code" ON "public"."workspaces" USING "btree" ("code");


--
-- Name: idx_workspaces_owner_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspaces_owner_user_id" ON "public"."workspaces" USING "btree" ("owner_user_id");


--
-- Name: idx_workspaces_parent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspaces_parent_id" ON "public"."workspaces" USING "btree" ("parent_id");


--
-- Name: idx_workspaces_room_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspaces_room_code" ON "public"."workspaces" USING "btree" ("room_code");


--
-- Name: idx_workspaces_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_workspaces_scope" ON "public"."workspaces" USING "btree" ("scope");


--
-- Name: idx_workspaces_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspaces_status" ON "public"."workspaces" USING "btree" ("status");


--
-- Name: idx_workspaces_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_workspaces_type" ON "public"."workspaces" USING "btree" ("type");


--
-- Name: activity_participations fk_activity_participation_activity; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."activity_participations"
    ADD CONSTRAINT "fk_activity_participation_activity" FOREIGN KEY ("activity_id") REFERENCES "public"."ops_activities"("id") ON DELETE RESTRICT;


--
-- Name: activity_participations fk_activity_participation_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."activity_participations"
    ADD CONSTRAINT "fk_activity_participation_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: agent_profit_share_records fk_agent_profit_share_agent; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."agent_profit_share_records"
    ADD CONSTRAINT "fk_agent_profit_share_agent" FOREIGN KEY ("agent_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: user_balance_transactions fk_balance_ledger_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_balance_transactions"
    ADD CONSTRAINT "fk_balance_ledger_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: lottery_bets fk_lottery_bet_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."lottery_bets"
    ADD CONSTRAINT "fk_lottery_bet_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: member_notifications fk_member_notification_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_notifications"
    ADD CONSTRAINT "fk_member_notification_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: member_payment_accounts fk_member_payment_account_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."member_payment_accounts"
    ADD CONSTRAINT "fk_member_payment_account_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: rebate_daily_records fk_rebate_daily_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."rebate_daily_records"
    ADD CONSTRAINT "fk_rebate_daily_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- Name: user_applications fk_user_application_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."user_applications"
    ADD CONSTRAINT "fk_user_application_user" FOREIGN KEY ("user_id") REFERENCES "public"."user"("user_id") ON DELETE RESTRICT;


--
-- PostgreSQL database dump complete
--
