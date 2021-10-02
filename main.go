package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"

	"github.com/fsnotify/fsnotify"
	"github.com/go-co-op/gocron"

	vault "github.com/hashicorp/vault/api"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace       = "db"
	exporter        = "exporter"
	vaultConfigName = "config"
	configFile      = "./config/config.yaml"
	timeout         = 15
	pingTimeout     = 10
)

var (
	vClient         *vault.Client
	vaultAddress    string
	jwtPath         string
	vaultPath       string
	vaultRole       string
	vaultSecretPath string
	vaultToken      string
	scheduler       *gocron.Scheduler
	sugar           *zap.SugaredLogger
	configuration   Configuration

	queryGaugeVec = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_value",
		Help:      "Value of Business metrics from Database",
	}, []string{"id", "database", "query", "column"})

	errorGaugeVec = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_error",
		Help:      "Result of last query, 1 if we have errors on running query",
	}, []string{"id", "database", "query"})

	durationGaugeVec = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "query_duration_seconds",
		Help:      "Duration of the query in seconds",
	}, []string{"id", "database", "query"})

	upGaugeVec = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: exporter,
		Name:      "up",
		Help:      "Database status, 1 if connect successful",
	}, []string{"id", "database"})
)

func main() {
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	core := ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	logger := zap.New(core, zap.AddCaller())
	sugar = logger.Sugar()

	prometheusConnection := "0.0.0.0:9103"

	//in case of vault usage these vars should be set
	jwtPath = envOrDie("JWT_PATH")
	vaultPath = envOrDie("VAULT_PATH")
	vaultRole = envOrDie("VAULT_ROLE")
	vaultSecretPath = envOrDie("VAULT_SECRET_PATH")

	vaultAddress = os.Getenv("VAULT_ADDR")
	vaultToken = os.Getenv("VAULT_TOKEN")

	scheduler = gocron.NewScheduler(time.UTC)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		sugar.Fatal(err)
	}
	//defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					updateConfig()

					err = watcher.Add(configFile)

					if err != nil {
						sugar.Errorf("Error on adding watcher: %v", err)
					}
				}
			case errWatch, ok := <-watcher.Errors:
				if !ok {
					return
				}

				sugar.Info("error:", errWatch)
			}
		}
	}()

	if err := watcher.Add(configFile); err != nil {
		sugar.Errorf("Error on adding watcher: %v", err)
	}

	updateConfig()

	sugar.Infof("Prometheus started and listen: %s", prometheusConnection)
	http.Handle("/metrics", promhttp.Handler())

	if err := http.ListenAndServe(prometheusConnection, nil); err != nil {
		sugar.Fatal("Fatal error on serving metrics:", err)
	}
}

func initVault(jwtPath, vaultPath, vaultRole string) error {
	if vaultToken != "" {
		vClient, _ = vault.NewClient(vault.DefaultConfig())

		return nil
	}

	vaultPath = fmt.Sprintf("auth/%s/login", vaultPath)
	bytes, err := ioutil.ReadFile(jwtPath)

	if err != nil {
		sugar.Error("error reading jwtPath: %v", err)

		return err
	}

	jwt := string(bytes)
	vaultClient, err := vault.NewClient(vault.DefaultConfig())

	if err != nil {
		sugar.Errorf("error creating vaultClient: %v", err)

		return err
	}

	if err := vaultClient.SetAddress(vaultAddress); err != nil {
		sugar.Errorf("error setting address on vaultClient: %v", err)

		return err
	}

	vaultResp, err := vaultClient.Logical().Write(
		vaultPath,
		map[string]interface{}{
			"role": vaultRole,
			"jwt":  jwt,
		})

	if err != nil {
		sugar.Errorf("Error get response from vaultClient: %v", err)

		return err
	}

	vClient = vaultClient
	vClient.SetToken(vaultResp.Auth.ClientToken)

	return nil
}

func readVaultValue(valueName string) string {
	if err := initVault(jwtPath, vaultPath, vaultRole); err != nil {
		sugar.Errorf("vault init failed: %v", err)
		os.Exit(1)
	}

	vaultResp, err := vClient.Logical().Read(vaultSecretPath)

	if err != nil {
		sugar.Errorf("vault get secret path failed: %v", err)
		os.Exit(1)
	}

	if _, ok := vaultResp.Data[valueName]; !ok {
		sugar.Errorf("vault get config failed: %v", err)
		os.Exit(1)
	}

	return fmt.Sprintf("%v", vaultResp.Data[valueName])
}

func envOrDie(env string) string {
	v, exists := os.LookupEnv(env)
	if !exists {
		sugar.Errorf("%s not set : missing parameter", env)
		os.Exit(1)
	}

	return v
}
