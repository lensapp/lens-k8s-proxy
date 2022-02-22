package main

import (
	"net"
	"os"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/proxy"
)

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")
	apiPrefix := os.Getenv("API_PREFIX")

	if apiPrefix == "" {
		apiPrefix = "/"
	}

	klog.Info("~~ Lens K8s Proxy ~~")
	klog.Info("kubeconfig: ", kubeconfig)
	klog.Info("api prefix: ", apiPrefix)

	config := genericclioptions.NewConfigFlags(false)
	config.KubeConfig = &kubeconfig
	clientConfig, err := config.ToRESTConfig()

	server, err := proxy.NewServer("", apiPrefix, "", nil, clientConfig, 0, false)

	if err != nil {
		klog.Fatal("failed to initialize proxy", err)

		os.Exit(1)
	}

	// Separate listening from serving so we can report the bound port
	// when it is chosen by os (eg: port == 0)
	var l net.Listener
	l, err = server.Listen("127.0.0.1", 0)
	if err != nil {
		klog.Fatal(err)

		os.Exit(1)
	}

	klog.Info("proxy listening on ", l.Addr().String())

	server.ServeOnListener(l)
}
