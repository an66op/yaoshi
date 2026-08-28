-- Application request ids are client retry keys.  The service-level lookup is
-- not enough under concurrent requests, so keep the first historical row and
-- enforce uniqueness at the database boundary for every non-empty key.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id, request_id
               ORDER BY id ASC
           ) AS duplicate_rank
    FROM user_applications
    WHERE request_id <> ''
)
UPDATE user_applications AS applications
SET request_id = ''
FROM ranked
WHERE applications.id = ranked.id
  AND ranked.duplicate_rank > 1;

DROP INDEX IF EXISTS idx_user_applications_request_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_applications_user_request
    ON user_applications (user_id, request_id)
    WHERE request_id <> '';
