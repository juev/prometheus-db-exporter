package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/go-co-op/gocron"

	consulapi "github.com/hashicorp/consul/api"
	vault "github.com/hashicorp/vault/api"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	namespace        = "db"
	exporter         = "exporter"
	vaultConfigName  = "config"
	vaultConsulToken = "token"
)

var (
	timeout          int
	err              error
	vClient          *vault.Client
	consulConfigPath string
	consulToken      string
	vaultAddress     string
	jwtPath          string
	vaultPath        string
	vaultRole        string
	vaultSecretPath  string
	vaultToken       string
	scheduler        *gocron.Scheduler
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime: "@timestamp",
			log.FieldKeyMsg:  "message"}})

	timeout = 10
	prometheusConnection := "0.0.0.0:9103"

	//in case of vault usage these vars should be set
	jwtPath = envOrDie("JWT_PATH")
	vaultPath = envOrDie("VAULT_PATH")
	vaultRole = envOrDie("VAULT_ROLE")
	vaultSecretPath = envOrDie("VAULT_SECRET_PATH")
	consulConfigPath = envOrDie("CONSUL_CONFIG_PATH")

	vaultAddress = os.Getenv("VAULT_ADDR")
	vaultToken = os.Getenv("VAULT_TOKEN")

	consulToken = readVaultValue(vaultConsulToken)

	scheduler = gocron.NewScheduler(time.UTC)
	scheduler.SetMaxConcurrentJobs(5, gocron.WaitMode)

	config := consulapi.DefaultConfig()
	config.Token = consulToken
	c, err := consulapi.NewClient(config)
	if err != nil {
		log.Errorf("Error creating consul client: %v", err)
		os.Exit(2)
	}
	kv := c.KV()
	ch := make(chan string)

	go subscribeToChanges(consulConfigPath, ch, kv)
	go updateConfig(ch)

	log.Infof("Prometheus started and listen: %s", prometheusConnection)
	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(prometheusConnection, nil)
	if err != nil {
		log.Fatalln("Fatal error on serving metrics:", err)
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
		log.Errorf("error reading jwtPath: %v", err)
		return err
	}
	jwt := string(bytes)
	vaultClient, err := vault.NewClient(vault.DefaultConfig())
	if err != nil {
		log.Errorf("error creating vaultClient: %v", err)
		return err
	}
	err = vaultClient.SetAddress(vaultAddress)
	if err != nil {
		log.Errorf("error setting address on vaultClient: %v", err)
		return err
	}
	vaultResp, err := vaultClient.Logical().Write(
		vaultPath,
		map[string]interface{}{
			"role": vaultRole,
			"jwt":  jwt,
		})
	if err != nil {
		log.Errorf("Error get response from vaultClient: %v", err)
		return err
	}
	vClient = vaultClient
	vClient.SetToken(vaultResp.Auth.ClientToken)
	return nil
}

func readVaultValue(valueName string) string {
	if err := initVault(jwtPath, vaultPath, vaultRole); err != nil {
		log.Errorf("vault init failed: %v", err)
		os.Exit(1)
	}
	vaultResp, err := vClient.Logical().Read(vaultSecretPath)
	if err != nil {
		log.Errorf("vault get secret path failed: %v", err)
		os.Exit(1)
	}
	_, ok := vaultResp.Data[valueName]
	if !ok {
		log.Errorf("vault get config failed: %v", err)
		os.Exit(1)
	}
	return fmt.Sprintf("%v", vaultResp.Data[valueName])
}

func envOrDie(env string) string {
	v, exists := os.LookupEnv(env)
	if !exists {
		log.Error(fmt.Errorf("%s not set", env), ": missing parameter")
		os.Exit(1)
	}
	return v
}
