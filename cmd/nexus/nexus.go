package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"

	"github.com/empirefox/nexus"
	"go.uber.org/zap"
)

var (
	config = flag.String("config", "$HOME/.hybrid/nexus.json", "config path")
	bind   = flag.String("bind", ":18888", "bind address")
)

type NexusServer struct {
	n *nexus.Nexus
}

func (s *NexusServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.n.ConnectSubscribe(w, r)
		return
	}

	switch r.URL.Path {
	case "/groups":
		s.n.GetGroups(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func main() {
	log, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	c, err := loadConfig(*config)
	if err != nil {
		log.Fatal("loadConfig", zap.Error(err))
	}

	n, err := nexus.NewNexus(log, *c)
	if err != nil {
		log.Fatal("NewNexus", zap.Error(err))
	}

	n.Start()
	s := &http.Server{
		Addr:    ":18888",
		Handler: &NexusServer{n},
	}

	err = s.ListenAndServe()
	if err != nil {
		log.Fatal("ListenAndServe", zap.Error(err))
	}
}

func loadConfig(p string) (*nexus.Config, error) {
	cf, err := os.Open(os.ExpandEnv(p))
	if err != nil {
		return nil, err
	}

	var c nexus.Config
	err = json.NewDecoder(cf).Decode(&c)
	return &c, err
}
