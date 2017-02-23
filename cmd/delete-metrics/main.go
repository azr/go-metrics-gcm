package main

import (
	"context"
	"flag"
	"log"
	"os"

	"golang.org/x/oauth2/google"

	cloudmonitoring "google.golang.org/api/monitoring/v3"
)

func main() {

	project := flag.String("project", os.Getenv("GOOGLE_CLOUD_PROJECT"), "GCP project")
	flag.Parse()

	metrics := flag.Args()
	if len(metrics) == 0 {
		log.Fatal("no metric to delete. usage: ./delete-metric -project proj-name metric1 [metric2 ...]")
	}
	if *project == "" {
		log.Fatal("Empty project ?")
	}

	ctx := context.Background()
	client, err := google.DefaultClient(ctx, cloudmonitoring.MonitoringScope)
	if err != nil {
		log.Fatal("Failed to get cloudmonitoring default client:", err)
	}
	s, err := cloudmonitoring.New(client)
	if err != nil {
		log.Fatal("Failed to get cloudmonitoring default service:", err)
	}
	*project = "projects/" + *project
	for _, metric := range metrics {
		_, err := s.Projects.MetricDescriptors.Delete(*project + "/metricDescriptors/" + metric).Do()
		if err != nil {
			log.Print("Failed to delete metric: ", err)
		} else {
			log.Print("Deleted ", metric)
		}
	}
}
