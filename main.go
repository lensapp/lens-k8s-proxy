package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
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
	proxyCert := os.Getenv("PROXY_CERT")
	proxyKey := os.Getenv("PROXY_KEY")

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

	l, err := getListener(proxyCert, proxyKey)

	if err != nil {
		klog.Fatal("failed to get listener", err)

		os.Exit(1)
	}

	fmt.Printf("starting to serve on %s\n", l.Addr().String())

	go func() {
		err := server.ServeOnListener(l)

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

const proxyAddr = "127.0.0.1:0"

func getListener(proxyCert string, proxyKey string) (net.Listener, error) {
	if proxyCert == "" || proxyKey == "" {
		return net.Listen("tcp", proxyAddr)
	}

	cert, err := tls.X509KeyPair([]byte(proxyCert), []byte(proxyKey))
	if err != nil {
		klog.Fatal(err)

		os.Exit(1)
	}

	return tls.Listen("tcp", proxyAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
}
