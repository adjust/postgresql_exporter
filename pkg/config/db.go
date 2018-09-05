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
	Workers() int
	Queries() []Query
	InstanceName() string
	Labels() map[string]string
	ApplicationName() string
}

// DbConfig describes database to get metrics from
type DbConfig struct {
	Host             string            `yaml:"host"`
	Port             uint16            `yaml:"port"`
	User             string            `yaml:"user"`
	Password         string            `yaml:"password"`
	Dbname           string            `yaml:"dbname"`
	Sslmode          string            `yaml:"sslmode"`
	QueryFiles       []string          `yaml:"queryFiles"`
	LabelsMap        map[string]string `yaml:"labels"`
	WorkersNumber    int               `yaml:"workers"`
	StatementTimeout time.Duration     `yaml:"statementTimeout"`
	IsNotPg          bool              `yaml:"isNotPg"`

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

// InstanceName returns instance name
func (d *DbConfig) InstanceName() string {
	return fmt.Sprintf("%s:%d", d.Host, d.Port)
}

// Workers returns number of workers for the db
func (d *DbConfig) Workers() int {
	return d.WorkersNumber
}

// Queries returns db queries
func (d *DbConfig) Queries() []Query {
	return d.queries
}

// Labels returns db labels
func (d *DbConfig) Labels() map[string]string {
	return d.LabelsMap
}

func (d *DbConfig) ApplicationName() string {
	return applicationName
}
