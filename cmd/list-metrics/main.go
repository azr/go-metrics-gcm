package main

import (
	"context"
	"flag"
	"log"
	"os"

	"fmt"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	cloudmonitoring "google.golang.org/api/monitoring/v3"
)

func main() {
	project := flag.String("project", os.Getenv("GOOGLE_CLOUD_PROJECT"), "GCP project")
	filter := flag.String("filter", `metric.type = starts_with("custom.googleapis.com/")`, "Filter on kind of metric")
	fields := flag.String("fields", `metricDescriptors(description,type,labels)`, "Filter to only get fields of metric ")
	flag.Parse()

	ctx := context.Background()
	client, err := google.DefaultClient(ctx, cloudmonitoring.MonitoringScope)
	if err != nil {
		log.Fatal("Failed to get cloudmonitoring default client:", err)
	}
	s, err := cloudmonitoring.New(client)
	if err != nil {
		log.Fatal("Failed to get cloudmonitoring default service:", err)
	}

	if *project == "" {
		log.Fatal("Empty project ?")
	}

	q := s.Projects.MetricDescriptors.List("projects/" + *project)
	if *filter != "" {
		q.Filter(*filter)
	}
	if *fields != "" {
		q.Fields(googleapi.Field(*fields))
	}
	resp, err := q.Do()
	if err != nil {
		log.Fatal("Failed to list descriptors: ", err)
	}
	b, _ := resp.MarshalJSON()
	fmt.Print(string(b))
}
