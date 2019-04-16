package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/pgtype"

	"github.com/adjust/postgresql_exporter/pkg/config"
)

//DbInterface describes Db methods
type DbInterface interface {
	SetStatementTimeout(time.Duration) error
	Exec(string) ([]map[string]interface{}, error)
	PgVersion() config.PgVersion
	Close() error
}

const queryCanceled = "57014"

// ErrQueryTimeout describes statement timeout error
var ErrQueryTimeout = errors.New("canceled due to statement timeout")

// Db describes database
type Db struct {
	version config.PgVersion
	db      *pgx.Conn
}

// New creates new instance of database connection
func New(dbConfig config.DbConfig) (*Db, error) {
	var version config.PgVersion

	cfg := pgx.ConnConfig{
		Host:                 dbConfig.Host,
		Port:                 dbConfig.Port,
		Database:             dbConfig.Dbname,
		User:                 dbConfig.User,
		Password:             dbConfig.Password,
		RuntimeParams:        map[string]string{"application_name": dbConfig.ApplicationName(), "client_encoding": "UTF8"},
		PreferSimpleProtocol: true,
	}

	if dbConfig.IsNotPg {
		cfg.CustomConnInfo = func(_ *pgx.Conn) (*pgtype.ConnInfo, error) {
			connInfo := pgtype.NewConnInfo()
			connInfo.InitializeDataTypes(map[string]pgtype.OID{
				"int2":    pgtype.Int2OID,
				"int4":    pgtype.Int4OID,
				"int8":    pgtype.Int8OID,
				"name":    pgtype.NameOID,
				"oid":     pgtype.OIDOID,
				"text":    pgtype.TextOID,
				"varchar": pgtype.VarcharOID,
			})

			return connInfo, nil
		}
	}

	dbConn, err := pgx.Connect(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not init db: %v", err)
	}

	if ver, ok := dbConn.RuntimeParams["server_version"]; ok && !dbConfig.IsNotPg {
		version = config.ParseVersion(ver)
	} else {
		version = config.NoVersion
	}

	if err != nil {
		return nil, fmt.Errorf("could not open connection: %v", err)
	}

	if !dbConfig.IsNotPg {
		if err := dbConn.Ping(context.Background()); err != nil {
			return nil, fmt.Errorf("could not ping db: %v", err)
		}
	}

	return &Db{
		db:      dbConn,
		version: version,
	}, nil
}

// Exec executes the query
func (d *Db) Exec(query string) ([]map[string]interface{}, error) {
	values := make([]map[string]interface{}, 0)

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query error: %v", err)
	}
	defer rows.Close()

	var columnNames []pgx.FieldDescription
	for rows.Next() {
		if rErr := rows.Err(); rErr != nil {
			return nil, fmt.Errorf("query error: %v", rErr)
		}

		if columnNames == nil {
			columnNames = rows.FieldDescriptions()
		}
		rawData, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("could not fetch values: %v", err)
		}

		row := make(map[string]interface{})
		for colId, column := range columnNames {
			row[column.Name] = rawData[colId]
		}

		values = append(values, row)
	}

	if rErr := rows.Err(); rErr != nil {
		pgErr, ok := rErr.(pgx.PgError)
		if !ok {
			return nil, fmt.Errorf("query error: %v", rErr)
		}

		if pgErr.Code == queryCanceled && strings.Contains(pgErr.Message, "statement timeout") {
			return nil, ErrQueryTimeout
		}

		return nil, fmt.Errorf("query error: %v - %T", rErr, rErr)
	}

	return values, nil
}

// SetStatementTimeout sets statement timeout
func (d *Db) SetStatementTimeout(duration time.Duration) error {
	_, err := d.db.Exec(fmt.Sprintf("set statement_timeout=%.0f", duration.Seconds()*1000))

	return err
}

// PgVersion returns Postgresql version
func (d *Db) PgVersion() config.PgVersion {
	return d.version
}

// Close closes connection to the database
func (d *Db) Close() error {
	return d.db.Close()
}

// ToFloat64 converts interface{} value to a float64 value
func ToFloat64(t interface{}) (float64, error) {
	var res float64
	switch v := t.(type) {
	case *pgtype.Numeric:
		err := v.AssignTo(&res)
		return res, err
	case int8:
		return float64(v), nil
	case int32:
		return float64(v), nil
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
	case int, int8, int16, int32, int64, float64, uint, uint8, uint16, uint32, uint64:
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
		if str, ok := v.(fmt.Stringer); ok {
			return str.String(), true
		}
		return "", false
	}
}
