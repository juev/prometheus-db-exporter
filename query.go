package main

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/godror/godror"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/lib/pq"

	"context"
	"math"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

func init() {
	queryGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_value",
		Help:      "Value of Business metrics from Database",
	}, []string{"id", "database", "query", "column"})

	prometheus.MustRegister(queryGaugeVec)

	errorGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_error",
		Help:      "Result of last query, 1 if we have errors on running query",
	}, []string{"id", "database", "query"})

	prometheus.MustRegister(errorGaugeVec)

	durationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_duration_seconds",
		Help:      "Duration of the query in seconds",
	}, []string{"id", "database", "query"})

	prometheus.MustRegister(durationGaugeVec)

	upGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "up",
		Help:      "Database status, 1 if connect successful",
	}, []string{"id", "database"})

	prometheus.MustRegister(upGaugeVec)
}

func execQuery(database *Database, query Query) {
	defer func(begun time.Time) {
		duration := time.Since(begun).Seconds()
		durationChannel <- DurationMetric{
			Id:       (*database).Id,
			Database: (*database).Database,
			Query:    query.Name,
			Value:    duration,
		}
	}(time.Now())

	if !pingDB(database, query.Name) {
		return
	}

	log.Infof("Start query `%v@%v`", database.connection, query.Name)

	// query db
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(queryTimeout)*time.Second)
	defer cancel()

	rows, err := database.pool.QueryContext(ctx, query.SQL)

	if err != nil {
		log.Errorf("Query '%v@%s' failed: %v", database.connection, query.Name, err)
		errorChannel <- ErrorMetric{
			Id:       (*database).Id,
			Database: (*database).Database,
			Query:    query.Name,
			Value:    1,
		}
		return
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Errorf("Error on closing rows: %v", err)
		}
	}()

	errorChannel <- ErrorMetric{
		Id:       (*database).Id,
		Database: (*database).Database,
		Query:    query.Name,
		Value:    0,
	}

	columns, _ := rows.Columns()
	count := len(columns)
	values := make([]interface{}, count)
	valuePtrs := make([]interface{}, count)

	for i := range columns {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		err = rows.Scan(valuePtrs...)
		if err != nil {
			log.Errorf("Error on scan: %v", err)
			break
		}

		for i, column := range columns {
			float, err := dbToFloat64(values[i])
			if err != nil {
				log.Errorf("Cannot convert value '%s' to float on query '%s': %v", values[i].(string), query.Name, err)
				errorChannel <- ErrorMetric{
					Id:       (*database).Id,
					Database: (*database).Database,
					Query:    query.Name,
					Value:    1,
				}
				continue
			}
			queryChannel <- QueryMetric{
				Id:       (*database).Id,
				Database: (*database).Database,
				Query:    query.Name,
				Column:   column,
				Value:    float,
			}
		}
	}
}

// Convert database.sql types to float64s for Prometheus consumption. Null types are mapped to NaN. string and []byte
// types are mapped as NaN and !ok
func dbToFloat64(t interface{}) (float64, error) {
	switch v := t.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case time.Time:
		return float64(v.Unix()), nil
	case []byte:
		// Try and convert to string and then parse to a float64
		strV := string(v)
		result, err := strconv.ParseFloat(strV, 64)
		if err != nil {
			log.Errorln("Could not parse []byte:", err)
			return math.NaN(), err
		}
		return result, nil
	case string:
		result, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Errorln("Could not parse string:", err)
			return math.NaN(), err
		}
		return result, nil
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	case nil:
		return math.NaN(), nil
	default:
		return math.NaN(), nil
	}
}

func pingDB(db *Database, query string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	if err := db.pool.PingContext(ctx); err != nil {
		log.Errorf("Error on check connection (ping) on db (%v@%v): %v", db.Id, query, err)
		return false
	}
	return true
}
