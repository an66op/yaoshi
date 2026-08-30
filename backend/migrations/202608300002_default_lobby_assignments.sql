-- Repair the legacy catalog once. Default shelf assignment is startup
-- configuration, not demo history, and is independent of whether a game is
-- enabled. Later operator edits must survive bootstrap/restarts: this repair
-- is versioned and the normal catalog seeder never updates existing games.
-- A soft-deleted shelf keeps its unique name, so DO NOTHING respects its
-- tombstone instead of reviving an intentionally removed category.
INSERT INTO public.lottery_lobby_categories (name, sort_order, created_at, updated_at)
VALUES
    ('彩票', 10, now(), now()),
    ('宾果', 20, now(), now()),
    ('PC', 30, now(), now()),
    ('六合彩', 40, now(), now())
ON CONFLICT (name) DO NOTHING;

-- Keep this inventory aligned with services.defaultLobbyPlacement. Taiwan
-- bingo belongs to 宾果; the other official titles follow the existing general
-- 彩票 games. Unknown/operator-created games are deliberately untouched.
WITH placements (game_id, lobby_category, lobby_sort_order) AS (
    VALUES
        ('speed-racing', '彩票', 1),
        ('speed-fly', '彩票', 2),
        ('speed-ssc', '彩票', 3),
        ('sg-fly', '彩票', 4),
        ('sg-ssc', '彩票', 5),
        ('fly-racing', '彩票', 6),
        ('au-lucky-5', '彩票', 7),
        ('au-lucky-10', '彩票', 8),
        ('official-fc3d', '彩票', 9),
        ('official-kl8', '彩票', 10),
        ('official-pl3', '彩票', 11),
        ('official-qxc', '彩票', 12),
        ('official-tw-super-lotto', '彩票', 13),
        ('official-tw-daily539', '彩票', 14),
        ('official-tw-lotto649', '彩票', 15),
        ('bingo-mark-six', '宾果', 1),
        ('bingo-racing-a', '宾果', 2),
        ('bingo-racing-b', '宾果', 3),
        ('bingo-ssc-1', '宾果', 4),
        ('bingo-ssc-2', '宾果', 5),
        ('bingo-ssc-3', '宾果', 6),
        ('bingo-ssc-4', '宾果', 7),
        ('official-tw-bingo', '宾果', 8),
        ('pc-canada', 'PC', 1),
        ('canada-28', 'PC', 2),
        ('canada-20', 'PC', 3),
        ('hong-kong-mark-six', '六合彩', 1),
        ('happy8-mark-six', '六合彩', 2),
        ('new-macau-mark-six', '六合彩', 3),
        ('old-macau-mark-six', '六合彩', 4)
)
UPDATE public.lottery_games AS game
SET lobby_category = placements.lobby_category,
    lobby_sort_order = CASE
        WHEN game.lobby_sort_order = 0 THEN placements.lobby_sort_order
        ELSE game.lobby_sort_order
    END,
    updated_at = now()
FROM placements
JOIN public.lottery_lobby_categories AS category
    ON category.name = placements.lobby_category
    AND category.deleted_at IS NULL
WHERE game.id = placements.game_id
    AND btrim(COALESCE(game.lobby_category, '')) = '';
