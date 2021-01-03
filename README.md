# prometheus-db-exporter

## Description

A [Prometheus](https://prometheus.io/) exporter for DB (Oracle, Postgres, Mysql). All requests are launched in parallel processes and do not block the HTTP entry point of Prometheus.

## Installation

### Docker

From the very beginning, you need to create a configuration file with a list of all connections and all the requests that you need, an example file in `config.yaml.template`.
Just copy to `config.yaml` and fill it.

You can run via Docker using an existing image. If you don't already have an Oracle server, you can run one locally in a container and then link the exporter to it.

```bash
docker run -d --name oracle -p 1521:1521 wnameless/oracle-xe-11g:16.04
docker run -d -v `pwd`/config.yaml:/config.yaml --name prometheus-db-exporter --link=oracle -p 9103:9103 juev/prometheus-db-exporter
```

Or just:

```bash
docker run --rm -v `pwd`/config.yaml:/config.yaml --name prometheus-db-exporter -p 9103:9103 juev/prometheus-db-exporter
```

## Metrics

The following metrics are exposed currently.

- db_exporter_dbmetric
- db_exporter_query_duration_seconds
- db_exporter_query_error
- db_exporter_up

Example:

```bash
# HELP db_exporter_query_duration_seconds Duration of the query in seconds
# TYPE db_exporter_query_duration_seconds gauge
db_exporter_query_duration_seconds{database="oracle",query="oracle_query"} 0.0262601
db_exporter_query_duration_seconds{database="postgres",query="postgres_query"} 0.0109653
# HELP db_exporter_query_error Result of last query, 1 if we have errors on running query
# TYPE db_exporter_query_error gauge
db_exporter_query_error{column="ID",database="oracle",query="oracle_query"} 0
db_exporter_query_error{column="NAME",database="oracle",query="oracle_query"} 0
db_exporter_query_error{column="id",database="postgres",query="postgres_query"} 0
db_exporter_query_error{column="name",database="postgres",query="postgres_query"} 0
# HELP db_exporter_query_value Value of Business metrics from Database
# TYPE db_exporter_query_value gauge
db_exporter_query_value{column="ID",database="oracle",query="oracle_query"} 1
db_exporter_query_value{column="NAME",database="oracle",query="oracle_query"} 222
db_exporter_query_value{column="id",database="postgres",query="postgres_query"} 1
db_exporter_query_value{column="name",database="postgres",query="postgres_query"} 222
# HELP db_exporter_up Database status
# TYPE db_exporter_up gauge
db_exporter_up{database="oracle"} 1
db_exporter_up{database="postgres"} 1
```

## Config file

```yaml
# Host, default value `0.0.0.0` (optional)
host: 0.0.0.0
# Port, default value `9103` (optional)
port: 9103
# QueryTimeout, default value `30` in seconds (optional)
queryTimeout: 5

# Array of databases and queries
databases:
    # Host, default value `127.0.0.1`, hostname for DB (optional)
  - host: 'dummy'
    # User (required)
    user: dummy
    # Port, default value `5432` (optional)
    port: 5432
    # Password (required)
    password: 'password'
    # Database name (required)
    database: dummy
    # Database rriver, default value `postgres` (optional, one of: postgres, oracle or mysql)
    driver: postgres
    # MaxIdleConns, default value `10` (optional)
    maxIdleConns: 10
    # MaxOpenConns, default value `10` (optional)
    maxOpenConns: 10
    # Aray of queries
    queries:
        # SQL, query (required)
      - sql: "select numbers1 from dummy"
        # SQL name (required)
        name: value1
        # Interval between queries, default value `1` in minites (optional)
        interval: 1
```
