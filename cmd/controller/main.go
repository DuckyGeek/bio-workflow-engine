package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	workflowv1 "github.com/yourusername/bio-workflow-engine/pkg/apis/v1"
	"github.com/yourusername/bio-workflow-engine/pkg/controller"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	fmt.Println("=== DEBUG: init() START ===")
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	fmt.Println("=== DEBUG: clientgoscheme added ===")
	utilruntime.Must(workflowv1.AddToScheme(scheme))
	fmt.Println("=== DEBUG: workflowv1 added ===")
}

func main() {
	fmt.Println("=== DEBUG: main() START ===")

	flag.Parse()
	fmt.Println("=== DEBUG: flags parsed ===")

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{
		Development: true,
	})))
	fmt.Println("=== DEBUG: logger set ===")

	fmt.Println("=== DEBUG: getting kubeconfig ===")
	config := ctrl.GetConfigOrDie()
	fmt.Println("=== DEBUG: kubeconfig obtained ===")

	// 创建 Manager，禁用默认的 metrics 服务器
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // 禁用默认 metrics 服务器
		},
	})
	if err != nil {
		fmt.Printf("=== ERROR: failed to create manager: %v ===\n", err)
		os.Exit(1)
	}
	fmt.Println("=== DEBUG: manager created ===")

	fmt.Println("=== DEBUG: setting up BioTask controller ===")
	if err = (&controller.BioTaskController{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("biotask-controller"),
	}).SetupWithManager(mgr); err != nil {
		fmt.Printf("=== ERROR: failed to setup BioTask controller: %v ===\n", err)
		os.Exit(1)
	}
	fmt.Println("=== DEBUG: BioTask controller setup complete ===")

	fmt.Println("=== DEBUG: setting up BioWorkflow controller ===")
	if err = (&controller.BioWorkflowController{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("bioworkflow-controller"),
	}).SetupWithManager(mgr); err != nil {
		fmt.Printf("=== ERROR: failed to setup BioWorkflow controller: %v ===\n", err)
		os.Exit(1)
	}
	fmt.Println("=== DEBUG: BioWorkflow controller setup complete ===")

	// 启动自定义 Metrics HTTP 服务器（监听 9090）
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println("Metrics server listening on :9090")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	fmt.Println("=== DEBUG: starting manager ===")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("=== ERROR: manager failed: %v ===\n", err)
		os.Exit(1)
	}
	fmt.Println("=== DEBUG: manager stopped ===")
}
