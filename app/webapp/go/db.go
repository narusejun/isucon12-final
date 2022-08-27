package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

var (
	dbHosts = []string{"133.152.6.84", "133.152.6.85"}
	dbs     = make([]*sqlx.DB, len(dbHosts))
)

func connectDatabase(host string) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=%s&multiStatements=%t&interpolateParams=true",
		getEnv("ISUCON_DB_USER", "isucon"),
		getEnv("ISUCON_DB_PASSWORD", "isucon"),
		host,
		getEnv("ISUCON_DB_PORT", "3306"),
		getEnv("ISUCON_DB_NAME", "isucon"),
		"Asia%2FTokyo",
		false,
	)
	dbx, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	return dbx, nil
}

func initDatabase() (err error) {
	for i, host := range dbHosts {
		dbs[i], err = connectDatabase(host)
		if err != nil {
			return err
		}
	}
	return nil
}
func selectDatabase(id int64) *sqlx.DB {
	return dbs[int(id)%len(dbs)]
}
func adminDatabase() *sqlx.DB {
	return selectDatabase(0)
}
