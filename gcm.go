//Provides a way to send your go-metrics to google cloud monitoring
//
// Histograms are not imoplemented yed because not available in custom metrics,
// see https://cloud.google.com/monitoring/api/metrics#value-types
//
// Timer is a Todo
package gcm

import (
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"github.com/rcrowley/go-metrics"
)

type Reporter struct {
	Registry metrics.Registry
	Interval time.Duration
	gcms     *cloudmonitoring.Service
	project  string
	source   string

	gauges map[string]struct{}
	//a fast and simple cache
}

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

func Monitor(r metrics.Registry, i time.Duration, s *cloudmonitoring.Service, project, source string) {
	NewReporter(r, i, s, project, source).Run()
}

func (self *Reporter) Run() {
	ticker := time.Tick(self.Interval)
	tss := cloudmonitoring.NewTimeseriesService(self.gcms)
	before := time.Now()
	for now := range ticker {
		now = time.Now()
		fmt.Printf("Now: %s", now.Format(time.RFC3339))
		reqs, err := self.BuildRequests(now.Format(time.RFC3339), before.Format(time.RFC3339), self.Registry)
		before = now
		if err != nil {
			log.Printf("ERROR sending gcm request %s", err)
			continue
		}

		for _, req := range reqs {
			wr := &cloudmonitoring.WriteTimeseriesRequest{}
			wr.CommonLabels["Source"] = self.source
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
			if m.Count() == 0 {
				return
			}
			p.Point.Int64Value = m.Count()
			p.Point.Start = end
		case metrics.Gauge:
			if m.Value() == 0 {
				return
			}
			self.CreateGauge(name, "int64")
			p.Point.Int64Value = m.Value()
			p.Point.Start = end
		case metrics.GaugeFloat64:
			if m.Value() == 0 {
				return
			}
			self.CreateGauge(name, "double")
			p.Point.DoubleValue = m.Value()
			p.Point.Start = end
		case metrics.Meter:
			m = m.Snapshot()
			if m.Count() == 0 {
				return
			}
			if m.RateMean() != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".mean"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: m.RateMean(),
						},
					},
				)
			}
			if m.Rate1() != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".1min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: m.Rate1(),
						},
					},
				)
			}
			if m.Rate5() != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".5min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: m.Rate5(),
						},
					},
				)
			}
			if m.Rate15() != 0 {
				pts = append(pts,
					&cloudmonitoring.TimeseriesPoint{
						TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
							Metric:  NamespacedName(name + ".15min"),
							Project: self.project,
						},
						Point: &cloudmonitoring.Point{
							Start:       end,
							End:         end,
							DoubleValue: m.Rate15(),
						},
					},
				)
			}
			return
		case metrics.Histogram:
			if m.Count() > 0 {
				log.Printf("Histograms are not available in custom metrics, see https://cloud.google.com/monitoring/api/metrics#value-types")
			}
			return
		case metrics.Timer:
			if m.Count() > 0 {
				log.Printf("Timers are not implemented yet")
			}
			return
		}
		pts = append(pts, p)
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
