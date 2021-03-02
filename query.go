package main

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/godror/godror"
	_ "github.com/lib/pq"

	"context"
	"math"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var (
	metricMap map[string]*prometheus.GaugeVec
)

func init() {
	metricMap = map[string]*prometheus.GaugeVec{
		"query": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "query_value",
			Help:      "Value of Business metrics from Database",
		}, []string{"database", "query", "column"}),
		"error": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "query_error",
			Help:      "Result of last query, 1 if we have errors on running query",
		}, []string{"database", "query", "column"}),
		"duration": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "query_duration_seconds",
			Help:      "Duration of the query in seconds",
		}, []string{"database", "query"}),
		"up": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "up",
			Help:      "Database status",
		}, []string{"database"}),
		"stats": prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporter,
			Name:      "stats",
			Help:      "Database stats (OpenConnections, InUse, Idle etc)",
		}, []string{"database", "metric"}),
	}
	for _, metric := range metricMap {
		prometheus.MustRegister(metric)
	}
}

func execQuery(database *Database, query Query) {

	defer func(begun time.Time) {
		duration := time.Since(begun).Seconds()
		metricMap["duration"].WithLabelValues(database.Database, query.Name).Set(duration)
	}(time.Now())

	// Reconnect if we lost connection
	if err := database.pool.Ping(); err != nil {
		log.Errorf("Error on connect to db (%s) with query (%s): %v", database.connection, query.Name, err)
		metricMap["up"].WithLabelValues(database.Database).Set(0)
		metricMap["error"].WithLabelValues(database.Database, query.Name, "fatal").Set(1)
		return
	}
	metricMap["up"].WithLabelValues(database.Database).Set(1)
	metricMap["error"].WithLabelValues(database.Database, query.Name, "fatal").Set(0)

	log.Infof("Start query `%v@%v`", database.connection, query.Name)

	// query db
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	rows, err := database.pool.QueryContext(ctx, query.SQL)
	if err != nil {
		log.Errorf("query '%s' failed: %v", query.Name, err)
		metricMap["error"].WithLabelValues(database.Database, query.Name, "fatal").Set(1)
		return
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Errorf("Error on closing rows: %v", err)
		}
	}()

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
			metricMap["error"].WithLabelValues(database.Database, query.Name, column).Set(0)
			float, err := dbToFloat64(values[i])
			if err != nil {
				log.Errorf("Cannot convert value '%s' to float on query '%s': %v", values[i].(string), query.Name, err)
				metricMap["error"].WithLabelValues(database.Database, query.Name, column).Set(1)
				return
			}
			metricMap["query"].With(prometheus.Labels{"database": database.Database, "query": query.Name, "column": column}).Set(float)
			metricMap["error"].WithLabelValues(database.Database, query.Name, column).Set(0)
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
