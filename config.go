package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"time"

	goora "github.com/sijms/go-ora/v2"

	"gopkg.in/yaml.v3"
)

// Configuration struct.
type Configuration []*Database

// Databases struct.
type Databases struct {
	ID      string `yaml:"id"`
	Queries []Query
}

// Database struct.
type Database struct {
	Dsn           string
	ID            string `yaml:"id"`
	Host          string `yaml:"host"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	Database      string `yaml:"database"`
	Port          int    `yaml:"port"`
	Driver        string `yaml:"driver"`
	MaxIdleCons   int    `yaml:"maxIdleCons"`
	MaxOpenCons   int    `yaml:"maxOpenCons"`
	ConnectString string `yaml:"connectString"`
	connection    string
	pool          *sql.DB
}

// Query struct.
type Query struct {
	SQL      string `yaml:"sql"`
	Name     string `yaml:"name"`
	Interval int    `yaml:"interval"`
	Timeout  int    `yaml:"timeout"`
}

func (d *Database) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type RawDatabase Database

	var defaults = RawDatabase{
		Host:          "127.0.0.1",
		Port:          5432,
		Driver:        "postgres",
		MaxIdleCons:   5,
		MaxOpenCons:   5,
		ConnectString: "",
	}

	if err := unmarshal(&defaults); err != nil {
		return err
	}

	*d = Database(defaults)

	return nil
}

func (raw *Query) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type RawQuery Query

	var defaults = RawQuery{
		Interval: 1,
		Timeout:  timeout,
	}

	if err := unmarshal(&defaults); err != nil {
		return err
	}

	*raw = Query(defaults)

	return nil
}

func updateConfig() {
	// update configuration from vault
	sugar.Info("Read database configuration from Vault")

	dbConfig := readVaultValue(vaultConfigName)

	sugar.Info("Read database configuration from configMap")

	tmpConfiguration := Configuration{}
	if err := yaml.Unmarshal([]byte(dbConfig), &tmpConfiguration); err != nil {
		sugar.Errorf("Cannot unmarshal key: %v", err)

		return
	}

	// Unmarshal yaml from config
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		sugar.Errorf("Cannot read config from file: %v", err)

		return
	}

	var databases []Databases

	if err := yaml.Unmarshal(data, &databases); err != nil {
		sugar.Errorf("Cannot unmarshal config file: %v", err)

		return
	}

	// Clear jobs
	scheduler.Stop()
	scheduler.Clear()

	// Close connections
	for _, database := range configuration {
		if database.pool != nil {
			if err := database.pool.Close(); err != nil {
				sugar.Errorf("Cannot close connection to %s : %v", database.Database, err)
			}
			database.pool = nil
		}
	}

	// Delete all metrics
	errorGaugeVec.Reset()
	queryGaugeVec.Reset()
	durationGaugeVec.Reset()
	upGaugeVec.Reset()

	configuration = tmpConfiguration

	// update connections for pool
	for _, database := range configuration {
		database.connection = fmt.Sprintf("%s:%d/%s", database.Host, database.Port, database.Database)
		// setup connection to database
		if database.Driver == "postgres" {
			database.Dsn = fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslmode=disable",
				database.User, database.Password, database.Host, database.Port, database.Database)
		} else if database.Driver == "oracle" {
			database.Dsn = goora.BuildUrl(database.Host,
				database.Port, database.Database, database.User, database.Password, nil)
		} else {
			database.Dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
				database.User, database.Password, database.Host, database.Port, database.Database)
		}
	}

	for _, database := range databases {
		for _, db := range configuration {
			if db.ID == database.ID {

				// Setup connection
				sugar.Infof("Setup connection for DB: %s", db.connection)
				db.pool, err = sql.Open(db.Driver, db.Dsn)

				if err != nil {
					sugar.Errorf("Error on setting connection to database (%s): %v", db.connection, err)

					continue
				}

				db.pool.SetMaxIdleConns(db.MaxIdleCons)
				db.pool.SetMaxOpenConns(db.MaxOpenCons)

				// Setup ping database
				if _, err := scheduler.Every(60).Seconds().Do(pingDB, db); err != nil {
					sugar.Errorf("Error on creating job pingDB (%v):, %v", db.Database, err)
				}

				// Setup queries
				for _, query := range database.Queries {
					tick := query.Interval * 60
					if query.Timeout > tick {
						tick = query.Timeout + 10
					}

					if _, err := scheduler.Every(tick).Seconds().Do(execQuery, db, query); err != nil {
						sugar.Errorf("Error creating job %v@%v: %v", db.Database, query.Name, err)

						continue
					}
				}

				break
			}
		}
	}

	scheduler.StartAsync()
}

func pingDB(database *Database) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pingTimeout)*time.Second)
	defer cancel()

	if err := database.pool.PingContext(ctx); err != nil {
		upGaugeVec.WithLabelValues(database.ID, database.Database).Set(0)

		return
	}

	upGaugeVec.WithLabelValues(database.ID, database.Database).Set(1)
}
