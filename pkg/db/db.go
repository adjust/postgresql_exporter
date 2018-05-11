package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq" // PostgreSQL SQL driver

	"github.com/ikitiki/postgresql_exporter/pkg/config"
)

//DbInterface describes Db methods
type DbInterface interface {
	SetStatementTimeout(time.Duration) error
	Exec(string) ([]map[string]interface{}, error)
	PgVersion() (config.PgVersion, error)
	Close() error
}

const queryCanceled = pq.ErrorCode("57014")

// ErrQueryTimeout describes statement timeout error
var ErrQueryTimeout = errors.New("canceled due to statement timeout")

// Db describes database
type Db struct {
	db *sql.DB
}

// New creates new instance of database connection
func New(connStr string) (*Db, error) {
	dbConn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("could not open connection: %v", err)
	}
	if err := dbConn.Ping(); err != nil {
		return nil, fmt.Errorf("could not ping db: %v", err)
	}

	return &Db{
		db: dbConn,
	}, nil
}

// Exec executes the query
func (d *Db) Exec(query string) ([]map[string]interface{}, error) {
	values := make([]map[string]interface{}, 0)

	rows, err := d.db.Query(query)
	if err != nil {
		pgErr := err.(*pq.Error)

		if pgErr.Code == queryCanceled && strings.Contains(pgErr.Message, "statement timeout") {
			return nil, ErrQueryTimeout
		}

		return nil, fmt.Errorf("query error: %v", err)
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("could not get columns: %v", err)
	}

	for rows.Next() {
		rawData := make([]interface{}, len(columnNames))
		pointers := make([]interface{}, len(columnNames))
		for id := range rawData {
			pointers[id] = &rawData[id]
		}

		if scanErr := rows.Scan(pointers...); scanErr != nil {
			return nil, fmt.Errorf("could not scan: %v", scanErr)
		}

		row := make(map[string]interface{})
		for colId, colName := range columnNames {
			row[colName] = rawData[colId]
		}

		values = append(values, row)
	}

	return values, nil
}

// SetStatementTimeout sets statement timeout
func (d *Db) SetStatementTimeout(duration time.Duration) error {
	_, err := d.db.Exec(fmt.Sprintf("set statement_timeout=%.0f", duration.Seconds()*1000))

	return err
}

// PgVersion returns Postgresql version
func (d *Db) PgVersion() (config.PgVersion, error) {
	var res config.PgVersion
	row := d.db.QueryRow("show server_version_num")
	if err := row.Scan(&res); err != nil {
		return res, fmt.Errorf("could not fetch row: %v", err)
	}

	return res, nil
}

// Close closes connection to the database
func (d *Db) Close() error {
	return d.db.Close()
}

// ToFloat64 converts interface{} value to a float64 value
func ToFloat64(t interface{}) (float64, error) {
	switch v := t.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case time.Time:
		return float64(v.Unix()), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case []byte:
		// Try and convert to string and then parse to a float64
		strV := string(v)
		result, err := strconv.ParseFloat(strV, 64)
		if err != nil {
			return math.NaN(), fmt.Errorf("could not parse []byte: %v", err)
		}
		return result, nil
	case string:
		result, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return math.NaN(), fmt.Errorf("could not parse string: %v", err)
		}
		return result, nil
	case nil:
		return math.NaN(), nil
	default:
		return math.NaN(), fmt.Errorf("unknown type %T", v)
	}
}

// ToString converts interface{} value to a string
func ToString(t interface{}) (string, bool) {
	switch v := t.(type) {
	case int64:
		return fmt.Sprintf("%v", v), true
	case float64:
		return fmt.Sprintf("%v", v), true
	case time.Time:
		return fmt.Sprintf("%v", v.Unix()), true
	case bool:
		return fmt.Sprintf("%v", v), true
	case nil:
		return "", true
	case []byte:
		// Try and convert to string
		return string(v), true
	case string:
		return v, true
	default:
		return "", false
	}
}
