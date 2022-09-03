#!/bin/sh
set -x
cd `dirname $0`

ISUCON_DB_HOST=${ISUCON_DB_HOST:-localhost}
ISUCON_DB_PORT=${ISUCON_DB_PORT:-3306}
ISUCON_DB_USER=${ISUCON_DB_USER:-isucon}
ISUCON_DB_PASSWORD=${ISUCON_DB_PASSWORD:-isucon}
ISUCON_DB_NAME=${ISUCON_DB_NAME:-isucon}
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-root}

sudo mariadb --default-character-set=utf8mb4  < 0_setup.sql


mariadb --default-character-set=utf8mb4 -u"$ISUCON_DB_USER" \
		-p"$ISUCON_DB_PASSWORD" \
		--host "$ISUCON_DB_HOST" \
		--port "$ISUCON_DB_PORT" \
		"$ISUCON_DB_NAME" < 1_schema.sql

mariadb --default-character-set=utf8mb4 -u"$ISUCON_DB_USER" \
		-p"$ISUCON_DB_PASSWORD" \
		--host "$ISUCON_DB_HOST" \
		--port "$ISUCON_DB_PORT" \
		"$ISUCON_DB_NAME" < 2_init.sql
