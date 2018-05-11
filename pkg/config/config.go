package config

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v2"
)

// ConfigInterface describes Config methods
type ConfigInterface interface {
	Load() error
	DbList() []string
	Db(string) DbConfig
}

// Config describes exporter config
type Config struct {
	configFile string
	dbs        map[string]DbConfig
}

// New creates new config
func New(filename string) *Config {
	cfg := Config{
		configFile: filename,
		dbs:        make(map[string]DbConfig, 0),
	}

	return &cfg
}

// Load loads the config
func (c *Config) Load() error {
	dbs := make(map[string]DbConfig)
	configDir, _ := path.Split(c.configFile)

	fp, err := os.Open(c.configFile)
	if err != nil {
		return fmt.Errorf("could not open file: %v", err)
	}
	defer fp.Close()

	decoder := yaml.NewDecoder(fp)
	if err := decoder.Decode(&dbs); err != nil {
		return fmt.Errorf("could not decode: %v", err)
	}

	for dbName, db := range dbs {
		if len(db.QueryFiles) == 0 {
			continue
		}

		for i, query := range db.QueryFiles {
			dbs[dbName].QueryFiles[i] = path.Join(configDir, query)
		}

		d := dbs[dbName]
		if err := d.LoadQueries(); err != nil {
			return fmt.Errorf("could not load db queries: %v", err)
		}
		if d.Workers <= 0 {
			d.Workers = 1
		}

		dbs[dbName] = d
	}

	c.dbs = dbs

	return nil
}

// DbList returns list of the databases
func (c *Config) DbList() []string {
	dbs := make([]string, 0)
	for dbName := range c.dbs {
		dbs = append(dbs, dbName)
	}

	return dbs
}

// Db returns the database config
func (c *Config) Db(dbName string) DbConfig {
	return c.dbs[dbName]
}
