CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY, 
    email TEXT NOT NULL, 
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_email_unigue UNIQUE (email)
);