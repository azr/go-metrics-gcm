//Package gcm Provides a way to send your go-metrics to google cloud monitoring
//
// Histograms are not implemented yet because not available in custom metrics,
// see https://cloud.google.com/monitoring/api/metrics#value-types
//
// Timer is to do
package gcm

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"github.com/rcrowley/go-metrics"
)

//Config contains configration to reports metrics to gcm
//
//Look at the Monitor func for an example usage.
type Config struct {
	Service *cloudmonitoring.Service // report to that service
	//Project measured.
	//you might want to set it to `projects/` + project name
	//when creating the reporter manually
	Project string
	//Labels to send along with queries,
	//usefull to know who sent what ex: ["source":$HOSTNAME].
	//labels are created on the first query,
	//if you start with no label then add some,
	//gcm will complain.
	//if you add a label and then remove one also
	//recreate metric to fix this
	Labels map[string]string

	// Type: Required. The monitored resource type. This field must match
	// the type field of a MonitoredResourceDescriptor object. For example,
	// the type of a Cloud SQL database is "cloudsql_database".
	// see https://cloud.google.com/monitoring/api/resources for types
	Type string
}

var (
	timersNotImplemented     sync.Once
	histogramsNotImplemented sync.Once
)

//Monitor starts a new single threaded monitoring process.
//See Config for parameters explanation (s, project, resourceType, labels).
//
//maxErrors is maximum number of retry if send fails, practical to not go over rate limits.
//
//Example:
//		client, err := google.DefaultClient(ctx, cloudmonitoring.MonitoringScope)
//		if err != nil {
//			log.Fatal("Failed to get cloudmonitoring default client:", err)
//		}
//		s, err := cloudmonitoring.New(client)
//		if err != nil {
//			log.Fatal("Failed to get cloudmonitoring default service:", err)
//		}
//
//		hostname, _ := os.Hostname()
//		if hostname == "" {
//			hostname = "unknown-hostname"
//		}
//		go googlecloudmetrics.Monitor(metrics.DefaultRegistry, 15*time.Second, 3, s, gcpProject, "api", map[string]string{"source": hostname, "service": service})
func Monitor(r metrics.Registry, tick time.Duration, maxErrors int, s *cloudmonitoring.Service, project, resourceType string, labels map[string]string) error {
	reporter := Config{
		Service: s,
		Project: "projects/" + project,
		Type:    resourceType,
	}
	ticker := time.NewTicker(tick)
	var errors int
	var err error
	for range ticker.C {
		err = reporter.Report(r)
		if err != nil {
			log.Printf("ERROR %d reporting metrics to gcm: %s", errors, err)
			errors++
		} else {
			errors = 0
		}

		if errors >= maxErrors {
			ticker.Stop()
			log.Printf("GCM got %d successive errors, leaving", maxErrors)
		}
	}
	return err
}

//Report every metric from r to gcm
func (config *Config) Report(r metrics.Registry) error {
	now := time.Now()
	reqs, err := config.buildTimeSeries(now.Format(time.RFC3339), r)
	if err != nil {
		log.Printf("ERROR sending gcm request %s", err)
		return err
	}

	wr := &cloudmonitoring.CreateTimeSeriesRequest{
		TimeSeries: reqs,
	}
	_, err = cloudmonitoring.NewProjectsTimeSeriesService(config.Service).Create(config.Project, wr).Do()
	return err
}

//DotSlashes makes golang metrics look like gcm metrics.
//
//ex : runtime.MemStats.StackSys -> runtime/MemStats/StackSys
func DotSlashes(name string) string {
	return strings.Replace(name, ".", "/", -1)
}

//customMetric names your custom metric
func customMetric(name string) string {
	return fmt.Sprintf("custom.googleapis.com/%s", DotSlashes(name))
}

//newTimeSeries creates a time serie named 'name' with labels from Config.
func (config *Config) newTimeSeries(name string) *cloudmonitoring.TimeSeries {
	return &cloudmonitoring.TimeSeries{
		Metric: &cloudmonitoring.Metric{
			Type:   customMetric(name),
			Labels: config.Labels,
		},
		Resource: &cloudmonitoring.MonitoredResource{
			Type: config.Type,
		},
	}
}

//buildTimeSeries returns an array of cloudmonitoring.TimeSeries containing
//eery metric to send to gcm.
func (config *Config) buildTimeSeries(start string, r metrics.Registry) (pts []*cloudmonitoring.TimeSeries, err error) {
	r.Each(func(name string, metric interface{}) {
		switch m := metric.(type) {
		case metrics.Counter:
			v := m.Count()
			if v == 0 {
				return
			}
			p := config.newTimeSeries(name)
			p.Points = append(p.Points, &cloudmonitoring.Point{
				Value: &cloudmonitoring.TypedValue{
					Int64Value: &v,
				},
				Interval: &cloudmonitoring.TimeInterval{
					EndTime: start,
				},
			})
			pts = append(pts, p)
		case metrics.Gauge:
			v := m.Value()
			if v == 0 {
				return
			}
			p := config.newTimeSeries(name)
			p.Points = append(p.Points, &cloudmonitoring.Point{
				Value: &cloudmonitoring.TypedValue{
					Int64Value: &v,
				},
				Interval: &cloudmonitoring.TimeInterval{
					EndTime: start,
				},
			})
			pts = append(pts, p)
		case metrics.GaugeFloat64:
			v := m.Value()
			if v == 0 {
				return
			}
			p := config.newTimeSeries(name)
			p.Points = append(p.Points, &cloudmonitoring.Point{
				Value: &cloudmonitoring.TypedValue{
					DoubleValue: &v,
				},
				Interval: &cloudmonitoring.TimeInterval{
					EndTime: start,
				},
			})
			pts = append(pts, p)
		case metrics.Meter:
			m = m.Snapshot()
			if v := m.RateMean(); v != 0 {
				p := config.newTimeSeries(name + ".mean")
				p.Points = append(p.Points, &cloudmonitoring.Point{
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: &v,
					},
					Interval: &cloudmonitoring.TimeInterval{
						EndTime: start,
					},
				})
				pts = append(pts, p)
			}
			if v := m.Rate1(); v != 0 {
				p := config.newTimeSeries(name + ".1min")
				p.Points = append(p.Points, &cloudmonitoring.Point{
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: &v,
					},
					Interval: &cloudmonitoring.TimeInterval{
						EndTime: start,
					},
				})
				pts = append(pts, p)
			}
			if v := m.Rate5(); v != 0 {
				p := config.newTimeSeries(name + ".5min")
				p.Points = append(p.Points, &cloudmonitoring.Point{
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: &v,
					},
					Interval: &cloudmonitoring.TimeInterval{
						EndTime: start,
					},
				})
				pts = append(pts, p)
			}
			if v := m.Rate15(); v != 0 {
				p := config.newTimeSeries(name + ".15min")
				p.Points = append(p.Points, &cloudmonitoring.Point{
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: &v,
					},
					Interval: &cloudmonitoring.TimeInterval{
						EndTime: start,
					},
				})
				pts = append(pts, p)
			}
		case metrics.Histogram:
			if m.Count() > 0 {
				histogramsNotImplemented.Do(func() {
					log.Printf("Histograms are not available in custom metrics, see https://cloud.google.com/monitoring/api/metrics#value-types")
				})
			}
		case metrics.Timer:
			if m.Count() > 0 {
				timersNotImplemented.Do(func() {
					log.Printf("Timers are not implemented yet")
				})
			}
		default:
			log.Printf("unknown metric ? %#v", metric)
		}
	})
	return
}
