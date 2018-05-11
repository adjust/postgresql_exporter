package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Column usage types
const (
	Discard ColumnUsage = iota // Ignore this column
	Label                      // Use this column as a label
	Counter                    // Use this column as a counter
	Gauge                      // Use this column as a gauge

	NoVersion PgVersion = -1
)

var (
	pgVerRegex = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)

	columnUsageMapping = map[string]ColumnUsage{
		"DISCARD": Discard,
		"LABEL":   Label,
		"COUNTER": Counter,
		"GAUGE":   Gauge,
	}
)

// ColumnUsage describes column usage
type ColumnUsage int

// PgVersion describes version in server_version_num format
type PgVersion int

// Metrics describe metrics
type Metrics map[string]Metric

// VerSQLs contain version specific SQLs
type VerSQLs []VerSQL

// Metric describes metric
type Metric struct {
	Usage       ColumnUsage `yaml:"usage"`
	Description string      `yaml:"description"`
}

// VerSQL describes PostgreSQL version specific SQL
type VerSQL struct {
	SQL    string
	MinVer PgVersion
	MaxVer PgVersion
}

// Query describes query
type Query struct {
	Name        string
	Metrics     Metrics `yaml:"metrics"`
	VerSQL      VerSQLs `yaml:"query"`
	NameColumn  string  `yaml:"nameColumn"`
	ValueColumn string  `yaml:"valueColumn"`
}

// UnmarshalYAML unmarshals the yaml
func (v *VerSQLs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	res := make(VerSQLs, 0)
	var val interface{}

	err := unmarshal(&val)
	if err != nil {
		return fmt.Errorf("could not unmarshal: %v", err)
	}

	switch val := val.(type) {
	case map[interface{}]interface{}:
		for k, v := range val {
			minPg, maxPg := parseVersionRange(fmt.Sprintf("%v", k))
			res = append(res, VerSQL{
				MinVer: minPg,
				MaxVer: maxPg,
				SQL:    v.(string),
			})
		}
	case interface{}:
		res = append(res, VerSQL{
			SQL: val.(string),
		})
	}

	*v = res

	return nil
}

// UnmarshalYAML unmarshals the yaml
func (c *ColumnUsage) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	unmarshal(&value)
	cu, ok := columnUsageMapping[value]
	if !ok {
		return fmt.Errorf("unknown usage: %v", value)
	}

	*c = cu

	return nil
}

// UnmarshalYAML unmarshals the yaml
func (m *Metrics) UnmarshalYAML(unmarshal func(interface{}) error) error {
	value := make(map[string]Metric, 0)
	queryMetrics := make([]map[string]Metric, 0)

	if err := unmarshal(&queryMetrics); err != nil {
		return err
	}

	for _, metrics := range queryMetrics {
		for name, descr := range metrics {
			value[name] = descr
		}
	}

	*m = value

	return nil
}

// UnmarshalYAML unmarshals the yaml
func (v *PgVersion) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	val := strings.Replace(str, ".", "", -1)
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return fmt.Errorf("could not convert string: %v", err)
	}
	*v = PgVersion(intVal)
	return nil
}

// PgVersion returns string representation of the version
func (v PgVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v/10000, (v/100)%100, v%100)
}

// Query returns query for the requested postgresql version
func (v VerSQLs) Query(version PgVersion) string {
	if version == NoVersion ||
		(len(v) == 1 && v[0].MaxVer == PgVersion(0) && v[0].MinVer == PgVersion(0)) {
		return v[0].SQL
	}

	for _, query := range v {
		if (version >= query.MinVer || query.MinVer == 0) && (version < query.MaxVer || query.MaxVer == 0) {
			return query.SQL
		}
	}

	return ""
}

func parseVersion(str string) PgVersion {
	var res int
	matches := pgVerRegex.FindStringSubmatch(str)
	if matches == nil {
		return PgVersion(res)
	}
	if matches[1] != "" {
		val, _ := strconv.Atoi(matches[1])
		res = val * 10000
		if val > 9 && matches[2] != "" {
			val, _ := strconv.Atoi(matches[2])
			res += val
		} else if matches[2] != "" {
			val, _ := strconv.Atoi(matches[2])
			res += val * 100
			if matches[3] != "" {
				val, _ := strconv.Atoi(matches[3])
				res += val
			}
		}
	}

	return PgVersion(res)
}

func parseVersionRange(str string) (PgVersion, PgVersion) {
	var min, max PgVersion
	if str == "" {
		return min, max
	}

	parts := strings.Split(str, "-")
	if len(parts) == 1 {
		min = parseVersion(parts[0])
		max = min
	} else {
		if parts[0] != "" {
			min = parseVersion(parts[0])
		}
		if parts[1] != "" {
			max = parseVersion(parts[1])
		}
	}

	return min, max
}
