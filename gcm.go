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

	"encoding/json"

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
	//to add or remove a label you need to recreate metric.
	Labels map[string]string

	//MonitoredRessource identifies the matchine/service/resource
	//that is monitored.
	//Different possible settings are defined here:
	//https://cloud.google.com/monitoring/api/resources
	//
	//setting a nil MonitoredRessource will default
	//to a "GlobalMonitoredResource" resource type.
	//
	//setting for MonitoredRessource can change without
	//you having to recreate the metric.
	MonitoredRessource *cloudmonitoring.MonitoredResource
}

var (
	timersNotImplemented     sync.Once
	histogramsNotImplemented sync.Once
	//GlobalMonitoredResource is the default monitored
	//resource that will be sent with the metrics
	//it doesn't allow much specification
	GlobalMonitoredResource = &cloudmonitoring.MonitoredResource{Type: "global"}
)

//Monitor starts a new single threaded monitoring process.
//See Config for parameters explanation (service, project, labels, monitoredRessource).
//
//maxErrors is maximum number of consecutive retries if send fails.
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
//		go googlecloudmetrics.Monitor(metrics.DefaultRegistry, 15*time.Second, 3, s, gcpProject, map[string]string{"source": hostname, "service": service}, nil)
func Monitor(r metrics.Registry, tick time.Duration, maxErrors int, service *cloudmonitoring.Service, project string, labels map[string]string, monitoredRessource *cloudmonitoring.MonitoredResource) error {
	if monitoredRessource == nil {
		monitoredRessource = GlobalMonitoredResource
	}
	reporter := Config{
		Service:            service,
		Project:            "projects/" + project,
		Labels:             labels,
		MonitoredRessource: monitoredRessource,
	}
	ticker := time.NewTicker(tick)
	var errors int
	var err error
	for range ticker.C {
		err = reporter.Report(r)
		if err != nil {
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

//Report every metric from registry to gcm
func (config *Config) Report(registry metrics.Registry) error {
	now := time.Now()
	reqs, err := config.buildTimeSeries(now.Format(time.RFC3339), registry)
	if err != nil {
		log.Printf("ERROR sending gcm request %s", err)
		return err
	}

	wr := &cloudmonitoring.CreateTimeSeriesRequest{
		TimeSeries: reqs,
	}
	_, err = cloudmonitoring.NewProjectsTimeSeriesService(config.Service).Create(config.Project, wr).Do()
	if err != nil {
		b, _ := json.Marshal(wr)
		log.Printf("ERROR reporting metrics to gcm: %s.Req: %s", err, b)
	}
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
	if config.MonitoredRessource == nil {
		//global doesn't allow much settings
		//but is a working default
		config.MonitoredRessource = GlobalMonitoredResource
	}
	return &cloudmonitoring.TimeSeries{
		Metric: &cloudmonitoring.Metric{
			Type:   customMetric(name),
			Labels: config.Labels,
		},
		Resource: config.MonitoredRessource,
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
