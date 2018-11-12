// Copyright © 2018 Cisco Systems, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
