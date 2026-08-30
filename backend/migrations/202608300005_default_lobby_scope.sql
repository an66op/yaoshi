-- Correct the initial catalog scope introduced by 202608300002 without
-- rewriting an applied migration. These eight catalog-only games have no
-- default lobby shelf. The other 22 built-in placements stay unchanged.
-- Match the previous category AND order so an operator's different placement
-- or explicit order is preserved. This versioned repair runs only once; later
-- assignments (including the same old tuple) are not reset on restart.
-- Catalog availability and existing room switches are independent of shelves.
WITH previous_defaults (game_id, lobby_category, lobby_sort_order) AS (
    VALUES
        ('official-fc3d', '彩票', 9),
        ('official-kl8', '彩票', 10),
        ('official-pl3', '彩票', 11),
        ('official-qxc', '彩票', 12),
        ('official-tw-super-lotto', '彩票', 13),
        ('official-tw-daily539', '彩票', 14),
        ('official-tw-lotto649', '彩票', 15),
        ('official-tw-bingo', '宾果', 8)
)
UPDATE public.lottery_games AS game
SET lobby_category = '',
    lobby_sort_order = 0,
    updated_at = now()
FROM previous_defaults
WHERE game.id = previous_defaults.game_id
  AND game.lobby_category = previous_defaults.lobby_category
  AND game.lobby_sort_order = previous_defaults.lobby_sort_order;
