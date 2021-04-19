#!/bin/bash
set -e

psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'bitburst'" | grep -q 1 || \
psql -U postgres <<-EOSQL
CREATE DATABASE "bitburst" WITH owner=postgres;
EOSQL
psql -U postgres --dbname bitburst -tc "
CREATE TABLE IF NOT EXISTS objects (
    id              INT          PRIMARY KEY,
    last_seen_at    TIMESTAMP
);"
