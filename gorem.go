package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

type configs map[string]config

type config struct {
	Entries []*struct {
		Path    string `json:"path"`
		Backend string `json:"backend"`
		proxy   http.Handler
	} `json:"entries"`
	Root     string `json:"root"`
	Address  string `json:"address"`
	FlagFile string `json:"flagfile"`
}

var configFile = flag.String("c", "config.json", "config file")

func loadConfig() (configs, error) {
	f, err := os.Open(*configFile)
	if err != nil {
		return nil, err
	}

	var c configs
	err = json.NewDecoder(f).Decode(&c)
	if err != nil {
		return nil, err
	}

	for _, v := range c {
		for _, entry := range v.Entries {
			u, err := url.Parse(entry.Backend)
			if err != nil {
				log.Println(err)
				continue
			}
			if u.Scheme == "" {
				entry.proxy = http.FileServer(http.Dir(entry.Backend))
			} else {
				if !strings.HasPrefix(entry.Path, "/") {
					entry.Path = path.Join(v.Root, "."+entry.Path)
				} else {
					entry.Path = path.Join(v.Root, entry.Path)
				}
				if !strings.HasSuffix(entry.Path, "/") {
					entry.Path += "/"
				}
				u.Path = ""
				u.RawQuery = ""
				u.Fragment = ""
				entry.Backend = u.String()
				entry.proxy = httputil.NewSingleHostReverseProxy(u)
			}
		}
	}
	return c, nil
}

func main() {
	flag.Parse()

	c, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP)
	go func() {
		for _ = range sc {
			log.Println("reloading configuration")
			c, err = loadConfig()
			if err != nil {
				log.Println(err)
			}
		}
	}()

	go func() {
		tc := time.Tick(10 * time.Second)
		for _ = range tc {
			for _, v := range c {
				if v.FlagFile == "" {
					continue
				}
				if b, err := ioutil.ReadFile(v.FlagFile); err == nil {
					k := strings.TrimSpace(string(b))
					os.Remove(v.FlagFile)
					if _, ok := c[k]; ok {
						if nc, err := loadConfig(); err == nil {
							log.Printf("[%s] updated configuration", k)
							c[k] = nc[k]
						}
					}
				}
			}
		}
	}()

	for k, v := range c {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			for _, entry := range v.Entries {
				if strings.HasPrefix(r.URL.Path, entry.Path) {
					forward := r.URL.Path[len(entry.Path)-1:]
					log.Printf("[%s] %s %s => %s%s", k, r.Method, r.URL.Path, entry.Backend, forward)
					r.URL.Path = forward
					r.Header.Set("X-Script-Name", forward)
					entry.proxy.ServeHTTP(w, r)
					return
				}
			}
			http.NotFound(w, r)
		})
		go func(mux *http.ServeMux, k string, v config) {
			log.Printf("[%s] server %s started", k, v.Address)
			if err = http.ListenAndServe(v.Address, mux); err != nil {
				log.Fatal(err)
			}
		}(mux, k, v)
	}

	quit := make(chan bool)
	<-quit
}
