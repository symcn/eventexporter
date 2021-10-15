package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/champly/eventexporter/pkg/kube"
	"github.com/symcn/api"
	"github.com/symcn/pkg/clustermanager/handler"
	"github.com/symcn/pkg/clustermanager/predicate"
	"github.com/symcn/pkg/clustermanager/workqueue"
	"github.com/symcn/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

func (ctrl *Controller) registryBeforAfterHandler() {
	// start metrics server
	go startMetricsServer(ctrl.ctx)

	ctrl.RegistryBeforAfterHandler(func(ctx context.Context, cli api.MingleClient) error {
		// build queue
		queue, err := workqueue.Complted(workqueue.NewWrapQueueConfig(cli.GetClusterCfgInfo().GetName(), ctrl)).NewQueue()
		if err != nil {
			return err
		}
		go queue.Start(ctx)

		// build labels & annotations cache
		ctrl.Lock()
		if _, ok := ctrl.metadataHandler[cli.GetClusterCfgInfo().GetName()]; !ok {
			ctrl.metadataHandler[cli.GetClusterCfgInfo().GetName()] = kube.NewMetadataHandler(ctrl.ctx, cli)
		}
		ctrl.Unlock()

		// add event handler
		cli.AddResourceEventHandler(
			&corev1.Event{},
			handler.NewResourceEventHandler(
				queue,
				handler.NewDefaultTransformNamespacedNameEventHandler(),
				predicate.NamespacePredicate("*"),
			),
		)

		return nil
	})
}

// startMetricsServer start http server with prometheus route
func startMetricsServer(ctx context.Context) {
	server := &http.Server{
		Addr: ":18080",
	}
	mux := http.NewServeMux()
	metrics.RegisterHTTPHandler(func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	})
	server.Handler = mux

	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !strings.EqualFold(err.Error(), "http: Server closed") {
				klog.Error(err)
				return
			}
		}
		klog.Info("http shutdown")
	}()
	<-ctx.Done()
	server.Shutdown(context.Background())
}
