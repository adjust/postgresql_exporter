package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// applicationName describes postgresql application name
const applicationName = "pg_prometheus_exporter"

// DbConfigInterface describes DbConfig methods
type DbConfigInterface interface {
	GetWorkers() int
	GetQueries() []Query
	ConnectionString() string
	InstanceName() string
	GetLabels() map[string]string
}

// DbConfig describes database to get metrics from
type DbConfig struct {
	Host                 string            `yaml:"host"`
	Port                 int               `yaml:"port"`
	User                 string            `yaml:"user"`
	Password             string            `yaml:"password"`
	Dbname               string            `yaml:"dbname"`
	Sslmode              string            `yaml:"sslmode"`
	QueryFiles           []string          `yaml:"queryFiles"`
	Labels               map[string]string `yaml:"labels"`
	Workers              int               `yaml:"workers"`
	SkipVersionDetection bool              `yaml:"skipVersionDetection"`
	StatementTimeout     time.Duration     `yaml:"statementTimeout"`

	queries []Query
}

// LoadQueries loads the queries from the QueryFiles
func (d *DbConfig) LoadQueries() error {
	queries := make([]Query, 0)

	for _, queryFile := range d.QueryFiles {
		fp, err := os.Open(queryFile)
		if err != nil {
			return fmt.Errorf("could not open file: %v", err)
		}

		fileQueries := make(map[string]Query)
		decoder := yaml.NewDecoder(fp)
		if err := decoder.Decode(&fileQueries); err != nil {
			fp.Close()
			return fmt.Errorf("could not decode %q: %v", queryFile, err)
		}

		for name, query := range fileQueries {
			query.Name = name
			queries = append(queries, query)
		}
		fp.Close()
	}
	d.queries = queries

	return nil
}

// ConnectionString returns connection string
func (d *DbConfig) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s&fallback_application_name=%s",
		d.User, d.Password, d.Host, d.Port, d.Dbname, d.Sslmode, applicationName)
}

// InstanceName returns instance name
func (d *DbConfig) InstanceName() string {
	return fmt.Sprintf("%s:%d", d.Host, d.Port)
}

// GetWorkers returns number of workers for the db
func (d *DbConfig) GetWorkers() int {
	return d.Workers
}

// GetQueries returns db queries
func (d *DbConfig) GetQueries() []Query {
	return d.queries
}

// GetLabels returns db labels
func (d *DbConfig) GetLabels() map[string]string {
	return d.Labels
}
