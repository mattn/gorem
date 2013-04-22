package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
)

type config struct {
	Entries []*struct {
		Path    string `json:"path"`
		Backend string `json:"backend"`
		proxy   *httputil.ReverseProxy
	} `json:"entries"`
	Root    string `json:"root"`
	Address string `json:"address"`
}

var configFile = flag.String("c", "config.json", "config file")

func main() {
	flag.Parse()
	f, err := os.Open(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	var c config
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range c.Entries {
		u, err := url.Parse(entry.Backend)
		if err != nil {
			log.Fatal(err)
		}
		if !strings.HasPrefix(entry.Path, "/") {
			entry.Path = path.Join(c.Root, "."+entry.Path)
		} else {
			entry.Path = path.Join(c.Root, entry.Path)
		}
		if !strings.HasSuffix(entry.Path, "/") {
			entry.Path += "/"
		}
		entry.proxy = httputil.NewSingleHostReverseProxy(u)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		for _, entry := range c.Entries {
			if strings.HasPrefix(r.URL.Path, entry.Path) {
				r.URL.Path = r.URL.Path[len(entry.Path)-1:]
				entry.proxy.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})

	if err = http.ListenAndServe(c.Address, nil); err != nil {
		log.Fatal(err)
	}
}
