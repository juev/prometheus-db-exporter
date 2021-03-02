package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Configuration struct
type Configuration struct {
	Databases []*Database
}

// Databases struct
type Databases struct {
	Database string `yaml:"database"`
	Queries  []Query
}

// Database struct
type Database struct {
	Dsn           string
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

// Query struct
type Query struct {
	SQL      string `yaml:"sql"`
	Name     string `yaml:"name"`
	Interval int    `yaml:"interval"`
}

var (
	configuration Configuration
)

func (raw *Database) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	*raw = Database(defaults)
	return nil
}

func (raw *Query) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type RawQuery Query
	var defaults = RawQuery{
		Interval: 1,
	}
	if err := unmarshal(&defaults); err != nil {
		return err
	}
	*raw = Query(defaults)
	return nil
}

func subscribeToChanges(key string, ch chan string, kv *consulapi.KV) {
	currentIndex := uint64(0)
	for {
		pair, meta, err := kv.Get(key, &consulapi.QueryOptions{
			//Datacenter:        "dc",
			WaitIndex: currentIndex,
			//RequireConsistent: true,
		})
		if err != nil {
			log.Errorf("Error read from KV: %v, %v", err.Error(), err)
			os.Exit(2)
		}
		if pair == nil || meta == nil {
			// Query won’t be blocked if key not found
			time.Sleep(1 * time.Second)
		} else {
			ch <- string(pair.Value)
			currentIndex = meta.LastIndex
		}
	}
}

func updateConfig(ch chan string) {
	for data := range ch {
		log.Info("Detected consul key change... updating configuration")

		// update configuration from vault
		log.Info("Read database configuration from Vault")
		dbConfig := readVaultValue(vaultConfigName)

		var tempConfiguration Configuration
		err = yaml.Unmarshal([]byte(dbConfig), &tempConfiguration.Databases)
		if err != nil {
			log.Errorf("Cannot unmarshal key: %v", err)
			break
		}
		configuration = tempConfiguration

		// update connections for pool
		for _, cDatabase := range configuration.Databases {
			cDatabase.connection = fmt.Sprintf("%s:%d/%s", cDatabase.Host, cDatabase.Port, cDatabase.Database)
			// connect to cDatabase
			if cDatabase.Driver == "postgres" {
				cDatabase.Dsn = fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslmode=disable", cDatabase.User, cDatabase.Password, cDatabase.Host, cDatabase.Port, cDatabase.Database)
			} else if cDatabase.Driver == "oracle" || cDatabase.Driver == "godror" {
				if cDatabase.ConnectString != "" {
					cDatabase.Dsn = fmt.Sprintf(`user="%s" password="%s" connectString="%s"`, cDatabase.User, cDatabase.Password, cDatabase.ConnectString)
				} else {
					cDatabase.Dsn = fmt.Sprintf(`user="%s" password="%s" connectString="%s:%d/%s"`, cDatabase.User, cDatabase.Password, cDatabase.Host, cDatabase.Port, cDatabase.Database)
				}
				cDatabase.Driver = "godror"
			} else {
				cDatabase.Dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", cDatabase.User, cDatabase.Password, cDatabase.Host, cDatabase.Port, cDatabase.Database)
			}
			log.Infoln("Open connection to DB:", cDatabase.connection)
			cDatabase.pool, err = sql.Open(cDatabase.Driver, cDatabase.Dsn)
			if err != nil {
				log.Errorf("Connection error to db: %v", err)
				break
			}
			defer func() {
				log.Info("Closing connection: %s", cDatabase.Database)
				err := cDatabase.pool.Close()
				if err != nil {
					log.Errorf("Error on closing connetcion: %v", err)
				}
			}()
			cDatabase.pool.SetConnMaxLifetime(5 * time.Minute)
			cDatabase.pool.SetMaxIdleConns(cDatabase.MaxIdleCons)
			cDatabase.pool.SetMaxOpenConns(cDatabase.MaxOpenCons)
		}

		// Unmarshal yaml from consul
		var databases []Databases
		err = yaml.Unmarshal([]byte(data), &databases)
		if err != nil {
			log.Errorf("Cannot unmarshal config file: %v", err)
			break
		}

		scheduler.Clear()
		for _, databaseA := range databases {
			var cDatabase Database
			for _, db := range configuration.Databases {
				if db.Database == databaseA.Database {
					cDatabase = *db
					break
				}
			}

			if cDatabase.Database == "" {
				break
			}

			_, err := scheduler.Every(60).Second().Do(getDBStats, &cDatabase)
			if err != nil {
				log.Errorf("Error on creating job: %v", err)
			}
			// create cron jobs for every query on cDatabase
			for _, query := range databaseA.Queries {
				_, err := scheduler.Every(query.Interval*60).Seconds().Do(execQuery, &cDatabase, query)
				if err != nil {
					log.Errorf("Error creating job: %v", err)
					break
				}
			}
		}

		scheduler.StartAsync()
	}
}

func getDBStats(db *Database) {
	stats := db.pool.Stats()
	metricMap["stats"].WithLabelValues(db.Database, "idle").Set(float64(stats.Idle))
	metricMap["stats"].WithLabelValues(db.Database, "inUse").Set(float64(stats.InUse))
	metricMap["stats"].WithLabelValues(db.Database, "openConnections").Set(float64(stats.OpenConnections))
	metricMap["stats"].WithLabelValues(db.Database, "waitCount").Set(float64(stats.WaitCount))
	metricMap["stats"].WithLabelValues(db.Database, "waitDuration").Set(float64(stats.WaitDuration))
}
