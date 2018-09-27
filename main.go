package main

import (
	"flag"
	"github.com/golang/glog"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/cisco-sso/pvwatch/pkg/signals"
)

var (
	metrics      = flag.String("metrics", ":9500", "The address to listen on for HTTP requests.")
	v            = flag.String("logLevel", "INFO", "Logging Level")
	devicePathRe = regexp.MustCompile(`WaitForAttach failed for Cinder disk.*devicePath is empty`)
	pvwatchCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pvwatch",
			Name:      "count",
			Help:      "Count of pvwatch processed events",
		},
		[]string{"event", "msg", "pod", "node", "err"},
	)
)

func init() {
	prometheus.MustRegister(pvwatchCount)
}

func startMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	glog.Errorf("Error starting metrics server %v", http.ListenAndServe(*metrics, nil))
}

func main() {
	stopCh := signals.SetupSignalHandler()
	flag.Parse()
	flag.Set("logtostderr", "true")

	kubeClient := clients()
	fac := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)

	controller := NewController(kubeClient,
		fac.Core().V1().Pods(),
		fac.Events().V1beta1().Events())
	go fac.Start(stopCh)
	go startMetrics()

	if err := controller.Run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %v", err)
	}
}

func clients() *kubernetes.Clientset {
	kubeconfig, err := restclient.InClusterConfig()
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %v", err)
	}
	return kubeClient
}
