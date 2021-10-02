package main

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/sijms/go-ora/v2"
)

func execQuery(database *Database, query Query) {

	if database.pool == nil {
		return
	}

	defer func(begun time.Time) {
		duration := time.Since(begun).Seconds()
		durationGaugeVec.WithLabelValues(database.ID, database.Database, query.Name).Set(duration)
	}(time.Now())

	sugar.Infof("Start query `%v@%v`", database.connection, query.Name)

	// query db
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(query.Timeout)*time.Second)
	defer cancel()

	rows, err := database.pool.QueryContext(ctx, query.SQL)

	if err != nil {
		sugar.Errorf("Query '%v@%s' failed: %v", database.connection, query.Name, err)
		errorGaugeVec.WithLabelValues(database.ID, database.Database, query.Name).Set(1)

		return
	}

	defer func(rows *sql.Rows) {
		if err := rows.Close(); err != nil {
			sugar.Errorf("Cannot close rows: %v", err)
		}
	}(rows)

	errorGaugeVec.WithLabelValues(database.ID, database.Database, query.Name).Set(0)

	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))

	for i := range columns {
		values[i] = new(sql.RawBytes)
	}

	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			sugar.Errorf("Error on scan: %v", err)

			continue
		}

		for i, column := range columns {
			float, errFloat := strconv.ParseFloat(string(*values[i].(*sql.RawBytes)), 64)

			if errFloat != nil {
				sugar.Errorf("Cannot convert value '%s' to float on query '%s': %v", values[i].(string), query.Name, err)
				errorGaugeVec.WithLabelValues(database.ID, database.Database, query.Name).Set(1)
				queryGaugeVec.DeleteLabelValues(database.ID, database.Database, query.Name, column)

				continue
			}

			queryGaugeVec.WithLabelValues(database.ID, database.Database, query.Name, column).Set(float)
		}
	}

	if err := rows.Close(); err != nil {
		sugar.Errorf("Error on closing rows: %v", err)
	}
}
