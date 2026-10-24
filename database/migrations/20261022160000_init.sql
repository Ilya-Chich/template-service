-- +goose Up
CREATE TABLE dummies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE CHECK (name ~ '[^[:space:]]'),
    note TEXT NOT NULL DEFAULT ''
);
-- +goose Down
DROP TABLE dummies;
