package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/proxy"
)

// These get overridden at build time: -X main.Version=$VERSION
var (
	Version = "0.0.1"
	Commit  = ""
)

type VersionData struct {
	Version string `json:"gitVersion"`
	Commit  string `json:"gitCommit"`
}

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) > 0 && argsWithoutProg[0] == "version" {
		err := json.NewEncoder(os.Stdout).Encode(&VersionData{
			Version: Version,
			Commit:  Commit,
		})

		if err != nil {
			klog.Fatal("failed to marshal version data", err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	kubeconfigContext := os.Getenv("KUBECONFIG_CONTEXT")
	apiPrefix := os.Getenv("API_PREFIX")
	certPath := os.Getenv("CERT_PATH")

	if !strings.HasSuffix(apiPrefix, "/") {
		apiPrefix += "/"
	}

	done := make(chan os.Signal, 2)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("~~ Lens K8s Proxy, '%s' ~~\n", Version)
	fmt.Printf("kubeconfig: %s\n", kubeconfig)
	fmt.Printf("api prefix: %s\n", apiPrefix)

	config := genericclioptions.NewConfigFlags(false)
	config.KubeConfig = &kubeconfig

	if kubeconfigContext != "" {
		config.Context = &kubeconfigContext
	}

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

		l, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{cer},
		})

		if err != nil {
			klog.Fatal("failed to start listening", err)

			os.Exit(1)
		}
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
