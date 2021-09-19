# prometheus-db-exporter

## Description

A [Prometheus](https://prometheus.io/) exporter for DB (Oracle, Postgres, Mysql). All requests are launched in parallel processes and do not block the HTTP entry point of Prometheus.

## Metrics

The following metrics are exposed currently.

- db_exporter_query_value
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

## Configuration

Configuration stored in two separate buckets:

1. Hashicorp vault (storing database auth and connection strings)
2. Local config file (storing database id and queries), cn be used as configMap in k8s

### Config file (vault)

```yaml
---
# Array of databases
-
  # ID (required)
  id: dummy_dummy
  # Database name (required)
  database: dummy
  # Database driver, default value `postgres` (optional, one of: postgres, oracle or mysql)
  driver: oracle
  # Host, default value `127.0.0.1`, hostname for DB (optional)
  host: 'dummy'
  # Password (required)
  password: 'password'
  # Port, default value `5432` (optional)
  port: 5432
  # User (required)
  user: dummy
  # Connection String (optional)
  connectString: (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=5432))(CONNECT_DATA=(SERVICE_NAME=dummy)))
```

### Local config file

```yaml
---
-
  # Database name (required)
  id: dummy
  # Queries
  queries:
    -
      # sql query (required)
      sql: "select 1 from table"
      # query name (required)
      name: query_name
      # interval in minutes (optional, default = 1)
      interval: 1
```