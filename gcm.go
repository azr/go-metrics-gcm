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

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"github.com/rcrowley/go-metrics"
)

//Reporter reports your metrics to GC monitoring
type Reporter struct {
	Registry metrics.Registry         // what metrics to report ?
	Interval time.Duration            // report every ?
	gcms     *cloudmonitoring.Service // report to that service
	project  string
	source   string

	gauges map[string]struct{}
	//a fast and simple cache
}

var (
	timersNotImplemented     sync.Once
	histogramsNotImplemented sync.Once
)

//NewReporter instantiates a reporter that sends metrics to GC monitoring
//source will be set labels as ["0":source]
//to help identify the machine sending the data
func NewReporter(r metrics.Registry, i time.Duration, s *cloudmonitoring.Service, project, source string) *Reporter {
	return &Reporter{
		Registry: r,
		Interval: i,
		gcms:     s,
		project:  project,
		source:   source,
		gauges:   make(map[string]struct{}),
	}
}

//Monitor starts a new
func Monitor(r metrics.Registry, i time.Duration, s *cloudmonitoring.Service, project, source string) {
	NewReporter(r, i, s, project, source).Run()
}

func (self *Reporter) Run() {
	ticker := time.Tick(self.Interval)
	tss := cloudmonitoring.NewTimeseriesService(self.gcms)
	before := time.Now()
	for now := range ticker {
		now = time.Now()
		reqs, err := self.BuildRequests(now.Format(time.RFC3339), before.Format(time.RFC3339), self.Registry)
		before = now
		if err != nil {
			log.Printf("ERROR sending gcm request %s", err)
			continue
		}

		for _, req := range reqs {
			wr := &cloudmonitoring.WriteTimeseriesRequest{
				CommonLabels: map[string]string{
					"0": "source",
				},
			}
			req.TimeseriesDesc.Labels = map[string]string{
				"source": DotSlashes(self.source),
			}
			//Can only write one simple point at a time !
			wr.Timeseries = append(wr.Timeseries, req)
			_, err = tss.Write(self.project, wr).Do()
			if err != nil {
				log.Printf("ERROR sending metrics to gcm %s", err)
			}
		}
	}
}

func DotSlashes(name string) string {
	return strings.Replace(name, ".", "/", -1)
}
func NamespacedName(name string) string {
	return fmt.Sprintf("custom.cloudmonitoring.googleapis.com/%s", DotSlashes(name))
}

//https://cloud.google.com/monitoring/demos/wrap_write_lightweight_metric
//https://cloud.google.com/monitoring/api/metrics
func (self *Reporter) BuildRequests(
	start, end string,
	r metrics.Registry,
) (
	pts []*cloudmonitoring.TimeseriesPoint,
	err error,
) {
	r.Each(func(name string, metric interface{}) {
		p := &cloudmonitoring.TimeseriesPoint{
			TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
				Metric:  NamespacedName(name),
				Project: self.project,
			},
			Point: &cloudmonitoring.Point{
				Start: start,
				End:   end,
			},
		}
		switch m := metric.(type) {
		case metrics.Counter:
			v := m.Count()
			if v == 0 {
				return
			}
			p.Point.Int64Value = &v
			p.Point.Start = end
			pts = append(pts, p)
		case metrics.Gauge:
			v := m.Value()
			m = m.Snapshot()
			if v == 0 {
				return
			}
			self.CreateGauge(name, "int64")
			p.Point.Int64Value = &v
			p.Point.Start = end
			pts = append(pts, p)
		case metrics.GaugeFloat64:
			v := m.Value()
			if v == 0 {
				return
			}
			self.CreateGauge(name, "double")
			p.Point.DoubleValue = &v
			p.Point.Start = end
			pts = append(pts, p)
		case metrics.Meter:
			m = m.Snapshot()
			if v := m.Count(); v != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".count"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:      end,
							End:        end,
							Int64Value: &v,
						},
					},
				)
			}
			if v := m.RateMean(); v != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".mean"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: &v,
						},
					},
				)
			}
			if v := m.Rate1(); v != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".1min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: &v,
						},
					},
				)
			}
			if v := m.Rate5(); v != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".5min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: &v,
						},
					},
				)
			}
			if v := m.Rate15(); v != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".15min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: &v,
						},
					},
				)
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

func (r *Reporter) CreateGauge(name, valueType string) {
	if _, found := r.gauges[name]; found {
		return
	}
	g := &cloudmonitoring.MetricDescriptor{
		Name: NamespacedName(name),
		Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
			&cloudmonitoring.MetricDescriptorLabelDescriptor{
				Description: "",
				Key:         NamespacedName(name),
			},
		},
		TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
			MetricType: "gauge",
			ValueType:  valueType,
		},
		Project:     r.project,
		Description: "Created by go-metrics",
	}
	_, err := r.gcms.MetricDescriptors.Create(r.project, g).Do()
	if err != nil {
		r.gauges[name] = struct{}{}
	}
}
