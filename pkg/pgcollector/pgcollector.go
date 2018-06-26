package pgcollector

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ikitiki/postgresql_exporter/pkg/config"
	"github.com/ikitiki/postgresql_exporter/pkg/db"
)

const (
	internalMetricsNamespace = "pg_exporter"
	scrapeDurationMetricName = "last_scrape_duration_seconds"
	timeOutsMetricName       = "last_scrape_timeouts"
	errorsNumMetricName      = "last_scrape_errors"
)

var internalMetricsDescriptions = map[string]string{
	scrapeDurationMetricName: "Duration of the last scrape of metrics",
	timeOutsMetricName:       "Number of timed out statements",
	errorsNumMetricName:      "Number of errors during scraping",
}

// PgCollector describes PostgreSQL metrics collector
type PgCollector struct {
	sync.Mutex
	config   config.ConfigInterface
	timeOuts uint32
	errors   uint32
}

type workerJob struct {
	config.Query
	dbLabels  map[string]string
	pgVersion config.PgVersion
}

// New create new instance of the PostgreSQL metrics collector
func New() *PgCollector {
	return &PgCollector{}
}

// LoadConfig loads config
func (p *PgCollector) LoadConfig(cfg *config.Config) {
	p.config = cfg
}

func createMetric(job *workerJob, name string, constLabels prometheus.Labels, rawValue interface{}) (prometheus.Metric, error) {
	switch job.Metrics[name].Usage {
	case config.Counter:
		m := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   job.Name,
			Name:        name,
			Help:        job.Metrics[name].Description,
			ConstLabels: constLabels,
		})
		val, err := db.ToFloat64(rawValue)
		if err != nil {
			return nil, fmt.Errorf("could not convert to float64: %v", err)
		}

		m.Add(val)
		return m, nil
	case config.Gauge:
		m := prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   job.Name,
			Name:        name,
			Help:        job.Metrics[name].Description,
			ConstLabels: constLabels,
		})
		val, err := db.ToFloat64(rawValue)
		if err != nil {
			return nil, fmt.Errorf("could not convert to float64: %v", err)
		}

		m.Add(val)
		return m, nil
	}

	return nil, nil
}

func (p *PgCollector) worker(conn db.DbInterface, jobs chan *workerJob, res chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
	}()

jobs:
	for job := range jobs {
		sql := job.VerSQL.Query(job.pgVersion)
		if sql == "" {
			log.Printf("could not find proper %q query variant for postgresql version %q", job.Name, job.pgVersion)
			atomic.AddUint32(&p.errors, 1)
			continue
		}

		labelColumns := make([]string, 0)
		for metricName, metric := range job.Metrics {
			if metric.Usage == config.Label {
				labelColumns = append(labelColumns, metricName)
				continue
			}
		}

		rows, err := conn.Exec(sql)
		if err != nil {
			if err == db.ErrQueryTimeout {
				atomic.AddUint32(&p.timeOuts, 1)
			}
			atomic.AddUint32(&p.errors, 1)
			log.Printf("could not fetch metric %q: %v", job.Name, err)
			continue
		}
		for _, row := range rows {
			labels := make(map[string]string)

			for _, columnName := range labelColumns {
				val, ok := db.ToString(row[columnName])
				if !ok {
					log.Printf("could not convert metric column value(%v) to string", row[columnName])
					atomic.AddUint32(&p.errors, 1)
				}
				labels[columnName] = val
			}
			constLabels := mergeLabels(job.dbLabels, labels)

			if job.NameColumn != "" {
				metricName, ok := db.ToString(row[job.NameColumn])
				if !ok {
					log.Printf("could not convert %v to string", row[job.NameColumn])
					atomic.AddUint32(&p.errors, 1)
					continue jobs
				}

				m, err := createMetric(job, metricName, constLabels, row[job.ValueColumn])
				if err != nil {
					log.Printf("could not create metric: %v", err)
					atomic.AddUint32(&p.errors, 1)
					continue jobs
				}
				if m != nil {
					res <- m
				}
			} else {
				for colName, colValue := range row {
					if _, ok := labels[colName]; ok {
						continue
					}

					if _, ok := job.Metrics[colName]; !ok {
						continue
					}

					m, err := createMetric(job, colName, constLabels, colValue)
					if err != nil {
						log.Printf("could not create metric: %v", err)
						atomic.AddUint32(&p.errors, 1)
						continue jobs
					}
					if m != nil {
						res <- m
					}
				}
			}
		}
	}
}

// Collect implements Collect method of the Collector interface
func (p *PgCollector) Collect(metricsCh chan<- prometheus.Metric) {
	p.Lock()
	defer p.Unlock()
	defer func(start time.Time) {
		gm := prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: internalMetricsNamespace,
			Name:      scrapeDurationMetricName,
			Help:      internalMetricsDescriptions[scrapeDurationMetricName],
		})
		gm.Set(time.Since(start).Seconds())
		metricsCh <- gm

		cm := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: internalMetricsNamespace,
			Name:      timeOutsMetricName,
			Help:      internalMetricsDescriptions[timeOutsMetricName],
		})
		cm.Add(float64(atomic.LoadUint32(&p.timeOuts)))
		metricsCh <- cm

		cm = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: internalMetricsNamespace,
			Name:      errorsNumMetricName,
			Help:      internalMetricsDescriptions[errorsNumMetricName],
		})
		cm.Add(float64(atomic.LoadUint32(&p.errors)))
		metricsCh <- cm
	}(time.Now())

	atomic.StoreUint32(&p.timeOuts, 0)
	atomic.StoreUint32(&p.errors, 0)

	pgVersions := make(map[string]config.PgVersion)
	wg := &sync.WaitGroup{}

	dbPool := make(map[string][]db.DbInterface)
	dbJobs := make(map[string]chan *workerJob)

dbLoop:
	for _, dbName := range p.config.DbList() {
		dbConf := p.config.Db(dbName)
		workersCnt := dbConf.GetWorkers()
		dbInstance := dbConf.InstanceName()
		dbPool[dbName] = make([]db.DbInterface, workersCnt)
		dbJobs[dbName] = make(chan *workerJob, workersCnt)

		for i := 0; i < workersCnt; i++ {
			conn, err := db.New(dbConf.ConnectionString())
			if err != nil {
				log.Printf("could not create db instance: %v", err)
				atomic.AddUint32(&p.errors, 1)
				break dbLoop
			}

			if pgVer, ok := pgVersions[dbInstance]; !ok {
				if dbConf.SkipVersionDetection {
					pgVer = config.NoVersion
				} else {
					pgVer, err = conn.PgVersion()
					if err != nil {
						log.Printf("could not get postgresql version: %v", err)
						atomic.AddUint32(&p.errors, 1)
						break dbLoop
					}
				}
				pgVersions[dbInstance] = pgVer
			}

			if dbConf.StatementTimeout != 0 {
				if err := conn.SetStatementTimeout(dbConf.StatementTimeout); err != nil {
					log.Printf("could not set statement timeout for %s: %v", dbInstance, err)
					atomic.AddUint32(&p.errors, 1)
					break dbLoop
				}
			}

			dbPool[dbName][i] = conn

			wg.Add(1)
			go p.worker(conn, dbJobs[dbName], metricsCh, wg)
		}

		for _, query := range dbConf.GetQueries() {
			dbJobs[dbName] <- &workerJob{
				dbLabels:  dbConf.GetLabels(),
				Query:     query,
				pgVersion: pgVersions[dbInstance],
			}
		}
		close(dbJobs[dbName])
	}

	wg.Wait()
	for dbName, dbs := range dbPool {
		for id, dbConn := range dbs {
			if dbConn == nil {
				continue
			}

			if err := dbConn.Close(); err != nil {
				log.Fatalf("%d: could not close db connection for %q: %v", id, dbName, err)
			}
		}
	}
}

// Describe implements Describe method of the Collector interface
func (p *PgCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, dbName := range p.config.DbList() {
		dbConf := p.config.Db(dbName)
		for _, query := range dbConf.GetQueries() {
			for metricName, metric := range query.Metrics {
				if metric.Usage == config.Label ||
					metric.Usage == config.Discard {
					continue
				}
				ch <- prometheus.NewDesc(
					prometheus.BuildFQName(query.Name, "", metricName),
					metric.Description,
					[]string{},
					nil)
			}
		}
	}

	for name, description := range internalMetricsDescriptions {
		ch <- prometheus.NewDesc(
			prometheus.BuildFQName(internalMetricsNamespace, "", name),
			description, []string{}, nil)
	}
}

func mergeLabels(a, b map[string]string) prometheus.Labels {
	res := make(prometheus.Labels)
	for id, value := range a {
		res[id] = value
	}

	for id, value := range b {
		res[id] = value
	}

	return res
}
