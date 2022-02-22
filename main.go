package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/proxy"
)

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")
	apiPrefix := os.Getenv("API_PREFIX")
	certPath := os.Getenv("CERT_PATH")

	if apiPrefix == "" {
		apiPrefix = "/"
	}

	done := make(chan os.Signal, 2)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	signal.Notify(done, os.Interrupt, syscall.SIGKILL)

	fmt.Println("~~ Lens K8s Proxy ~~")
	fmt.Printf("kubeconfig: %s\n", kubeconfig)
	fmt.Printf("api prefix: %s\n", apiPrefix)

	config := genericclioptions.NewConfigFlags(false)
	config.KubeConfig = &kubeconfig
	clientConfig, err := config.ToRESTConfig()

	if err != nil {
		klog.Fatal("failed to initialize kubeconfig", err)

		os.Exit(1)
	}

	server, err := proxy.NewServer("", apiPrefix, "", nil, clientConfig, 0, true)

	if err != nil {
		klog.Fatal("failed to initialize proxy", err)

		os.Exit(1)
	}

	// Separate listening from serving so we can report the bound port
	// when it is chosen by os (eg: port == 0)
	var l net.Listener
	if certPath == "" {
		l, err = server.Listen("127.0.0.1", 0)
		if err != nil {
			klog.Fatal(err)

			os.Exit(1)
		}
	} else {
		cer, err := tls.LoadX509KeyPair(path.Join(certPath, "proxy.crt"), path.Join(certPath, "proxy.key"))
		if err != nil {
			klog.Fatal(err)

			os.Exit(1)
		}
		config := &tls.Config{Certificates: []tls.Certificate{cer}}
		l, err = tls.Listen("tcp", "127.0.0.1:0", config)
	}

	fmt.Printf("starting to serve on %s\n", l.Addr().String())

	go func() {
		err = server.ServeOnListener(l)

		if err != nil {
			klog.Fatal(err)

			os.Exit(1)
		}
	}()

	<-done

	fmt.Println("shutting down ...")

	l.Close()
	os.Exit(0)
}
