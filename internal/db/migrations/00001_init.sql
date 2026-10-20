-- +goose Up
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    oidc_subject  TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notes (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    author_id   BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_notes_updated_at ON notes (updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_notes_author_id ON notes (author_id) WHERE deleted_at IS NULL;

CREATE TABLE comments (
    id          BIGSERIAL PRIMARY KEY,
    note_id     BIGINT NOT NULL REFERENCES notes(id),
    content     TEXT NOT NULL,
    author_id   BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_comments_note_id ON comments (note_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE comments;
DROP TABLE notes;
DROP TABLE users;
