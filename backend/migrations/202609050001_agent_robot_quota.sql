ALTER TABLE workspaces
    ADD COLUMN robot_quota integer NOT NULL DEFAULT 10;

ALTER TABLE workspaces
    ADD CONSTRAINT chk_workspaces_robot_quota
    CHECK (robot_quota >= 0 AND robot_quota <= 10);
