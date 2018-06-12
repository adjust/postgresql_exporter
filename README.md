# PostgreSQL Metrics Exporter
[![Go Report Card](https://goreportcard.com/badge/github.com/ikitiki/postgresql_exporter)](https://goreportcard.com/report/github.com/ikitiki/postgresql_exporter)


Prometheus exporter for PostgreSQL metrics.<br>

## Features

- Use of multiple connections while fetching metrics
- Statement timeouts to cancel long-running queries
- Version-specific queries in the config file
- Connection labels

## Getting and running

Get:
```
    go get -u github.com/ikitiki/postgresql_exporter
```

Run:
```
    postgresql_exporter --config {path to config file}
```


## Config file
```
{connection name}: 
    host: {host}
    port: {port}
    user: {username}
    dbname: {db name}
    sslmode: {ssl mode}
    workers: {number of parallel connections to use}
    statementTimeout: {pg statement_timeout value for each connection}
    skipVersionDetection: {whether perform "show server_version_num" on connect or not; useful while gathering stats of connection poolers}
    labels: 
        {labels added to each metric in the "queryFiles"}
    queryFiles: 
        {use metric queries from files}
```

sample:
```
pg10:
    host: localhost
    port: 5432
    user: postgres
    dbname: test
    sslmode: disable
    workers: 5
    labels:
        ver: 10
    queryFiles:
        - "basic.yaml"
        - "pertable.yaml"
```

## Query file

sample:
first query will be using for postgresql >=10
the second query will be using for postgresql >=9.4 but <10 
```
pg_slots:
    query:
        10-: >-
            select
            slot_name,
            slot_type,
            active,
            case when not pg_is_in_recovery() then pg_current_wal_lsn() - restart_lsn end as current_lag_bytes
            from pg_replication_slots s
            order by s.slot_name
        9.4-10: >-
            select
            slot_name,
            slot_type,
            active,
            case when not pg_is_in_recovery() then pg_current_xlog_location() - restart_lsn end as current_lag_bytes
            from pg_replication_slots s
            order by s.slot_name
    metrics:
        - slot_name:
            usage: "LABEL"
            description: "Slot name"
        - slot_type:
            usage: "LABEL"
            description: "Slot type"
        - active:
            usage: "LABEL"
            description: "Is slot active"
        - current_lag_bytes:
            usage: "GAUGE"
            description: "Lag in bytes"
```

query will be used for all the postgresql versions:
```
relation_total_size:
    query: >-
        select
            n.nspname as schemaname,
            c.relname,
            pg_total_relation_size(c.oid) as inclusive_bytes,
            pg_relation_size(c.oid) as exclusive_bytes
        from pg_class c
        join pg_namespace n on c.relnamespace = n.oid
        where relkind = 'r'
        and n.nspname not in ('pg_toast', 'pg_catalog', 'information_schema')
    metrics:
        - schemaname:
            usage: "LABEL"
            description: "Schema of relation"
        - relname:
            usage: "LABEL"
            description: "Name of relation"
        - inclusive_bytes:
            usage: "GAUGE"
            description: "Size of table, including indexes and toast"
        - exclusive_bytes:
            usage: "GAUGE"
            description: "Size of table, excluding indexes and toast"
```


if you need to get metric names and values from the columns,
specify them in the "nameColumn" and "valueColumn" accordingly:
```
pg_settings:
    query:
        8.0-9.5: >-
            select
                name,
                case setting when 'off' then 0 when 'on' then 1 else setting::numeric end as setting
            from pg_settings
            where vartype IN ('bool', 'integer', 'real')
        9.5-: >-
            select
                name,
                case setting when 'off' then 0 when 'on' then 1 else setting::numeric end as setting,
                pending_restart
            from pg_settings
            where vartype IN ('bool', 'integer', 'real')
    nameColumn: "name"
    valueColumn: "setting"
    metrics:
        - pending_restart:
            usage: "LABEL"
            description: "if the value has been changed in the configuration file but needs a restart"
        - allow_system_table_mods:
            usage: "GAUGE"
            description: "Allows modifications of the structure of system tables"
        - archive_timeout:
            usage: "GAUGE"
            description: "Forces a switch to the next WAL file if a new file has not been started within N seconds"
...
```
