package main

import (
	"context"
	"flag"
	"log"
	"os"

	"fmt"

	"golang.org/x/oauth2/google"
	cloudmonitoring "google.golang.org/api/cloudmonitoring/v2beta2"
)

func main() {
	project := flag.String("project", os.Getenv("GOOGLE_CLOUD_PROJECT"), "GCP project")
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

	resp, err := s.MetricDescriptors.List(*project, &cloudmonitoring.ListMetricDescriptorsRequest{}).Do()
	if err != nil {
		log.Fatal("Failed to list descriptors: ", err)
	}

	b, _ := resp.MarshalJSON()
	fmt.Print(string(b))
}
