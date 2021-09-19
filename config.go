package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Configuration struct
type Configuration struct {
	Databases []*Database
}

// Databases struct
type Databases struct {
	Id      string `yaml:"id"`
	Queries []Query
}

// Database struct
type Database struct {
	Dsn           string
	Id            string `yaml:"id"`
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

func updateConfig() {
	// update configuration from vault
	log.Info("Read database configuration from Vault")
	dbConfig := readVaultValue(vaultConfigName)

	log.Info("Read database configuration from configMap")
	var configuration Configuration
	err = yaml.Unmarshal([]byte(dbConfig), &configuration.Databases)
	if err != nil {
		log.Errorf("Cannot unmarshal key: %v", err)
		return
	}

	// Unmarshal yaml from config
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Errorf("Cannot read config from file: %v", err)
		return
	}

	var databases []Databases
	err = yaml.Unmarshal(data, &databases)
	if err != nil {
		log.Errorf("Cannot unmarshal config file: %v", err)
		return
	}

	// Close channels for ending running goroutines
	if queryChannel != nil {
		close(queryChannel)
	}
	if errorChannel != nil {
		close(errorChannel)
	}
	if durationChannel != nil {
		close(durationChannel)
	}
	if upChannel != nil {
		close(upChannel)
	}

	// Create new channels
	queryChannel = make(chan QueryMetric, 1)
	errorChannel = make(chan ErrorMetric, 1)
	durationChannel = make(chan DurationMetric, 1)
	upChannel = make(chan UpMetric, 1)

	// Reset all metrics
	queryGaugeVec.Reset()
	errorGaugeVec.Reset()
	durationGaugeVec.Reset()
	upGaugeVec.Reset()

	// Run goprocesses
	go runQueryProcess(queryChannel)
	go runErrorProcess(errorChannel)
	go runDurationProcess(durationChannel)
	go runUpProcess(upChannel)

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
		log.Infoln("Setup connection for DB:", cDatabase.connection)
		cDatabase.pool, err = sql.Open(cDatabase.Driver, cDatabase.Dsn)
		if err != nil {
			log.Errorf("Error on setting connection to db (%s): %v", cDatabase.connection, err)
			upChannel <- UpMetric{
				Id:       (*cDatabase).Id,
				Database: (*cDatabase).Database,
				Value:    0,
			}
			continue
		}
		cDatabase.pool.SetConnMaxLifetime(5 * time.Minute)
		cDatabase.pool.SetMaxIdleConns(cDatabase.MaxIdleCons)
		cDatabase.pool.SetMaxOpenConns(cDatabase.MaxOpenCons)
	}

	scheduler.Clear()

	for _, databaseA := range databases {
		for _, db := range configuration.Databases {
			if db.Id == databaseA.Id {
				if err != nil {
					log.Errorf("Error on creating job pingDB (%v):, %v", db.Database, err)
				}
				for _, query := range databaseA.Queries {
					_, err := scheduler.Every(query.Interval*60).Seconds().Do(execQuery, db, query)
					if err != nil {
						log.Errorf("Error creating job %v@%v: %v", db.Database, query.Name, err)
						continue
					}
				}
				break
			}
		}
	}
	scheduler.StartAsync()
}

func runQueryProcess(c chan QueryMetric) {
	for val := range c {
		queryGaugeVec.WithLabelValues(val.Id, val.Database, val.Query, val.Column).Set(val.Value)
	}
}

func runErrorProcess(c chan ErrorMetric) {
	for val := range c {
		errorGaugeVec.WithLabelValues(val.Id, val.Database, val.Query).Set(val.Value)
	}
}

func runDurationProcess(c chan DurationMetric) {
	for val := range c {
		durationGaugeVec.WithLabelValues(val.Id, val.Database, val.Query).Set(val.Value)
	}
}

func runUpProcess(c chan UpMetric) {
	for val := range c {
		upGaugeVec.WithLabelValues(val.Id, val.Database).Set(val.Value)
	}
}
