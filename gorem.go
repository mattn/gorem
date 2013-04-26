package main

import (
	"encoding/json"
	"flag"
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

type Entry struct {
	Path    string `json:"path"`
	Backend string `json:"backend"`
	proxy   http.Handler
}

type Config struct {
	Entries  []*Entry `json:"entries"`
	Root     string   `json:"root"`
	Address  string   `json:"address"`
	FlagFile string   `json:"flagfile"`
}

type Configs map[string]Config

var configFile = flag.String("c", "config.json", "config file")

func setupEntries(c *Config) {
	for _, entry := range c.Entries {
		u, err := url.Parse(entry.Backend)
		if err != nil {
			log.Println(err)
			continue
		}
		if !strings.HasPrefix(entry.Path, "/") {
			entry.Path = path.Join(c.Root, "."+entry.Path)
		} else {
			entry.Path = path.Join(c.Root, entry.Path)
		}
		if !strings.HasSuffix(entry.Path, "/") {
			entry.Path += "/"
		}
		if u.Scheme == "" {
			if !strings.HasSuffix(entry.Backend, "/") {
				entry.Backend += "/"
			}
			entry.proxy = http.FileServer(http.Dir(entry.Backend))
		} else {
			u.Path = "/"
			u.RawQuery = ""
			u.Fragment = ""
			entry.Backend = u.String()
			entry.proxy = httputil.NewSingleHostReverseProxy(u)
		}
	}
}

func updateConfig(c *Config, name string) error {
	f, err := os.Open(*configFile)
	if err != nil {
		return err
	}

	var cl Configs
	err = json.NewDecoder(f).Decode(&cl)
	if err != nil {
		return err
	}

	for k := range cl {
		if k == name {
			continue
		}
		*c = cl[k]
		setupEntries(c)
	}
	return nil
}

func loadConfigs() (Configs, error) {
	f, err := os.Open(*configFile)
	if err != nil {
		return nil, err
	}

	var cl Configs
	err = json.NewDecoder(f).Decode(&cl)
	if err != nil {
		return nil, err
	}

	for _, c := range cl {
		setupEntries(&c)
	}
	return cl, nil
}

func main() {
	flag.Parse()

	cl, err := loadConfigs()
	if err != nil {
		log.Fatal(err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP)
	go func() {
		for _ = range sc {
			log.Println("reloading configuration")
			cl, err = loadConfigs()
			if err != nil {
				log.Println(err)
			}
		}
	}()

	go func() {
		tc := time.Tick(10 * time.Second)
		for _ = range tc {
			for k, c := range cl {
				if c.FlagFile == "" {
					continue
				}
				if _, err := os.Stat(c.FlagFile); err == nil {
					os.Remove(c.FlagFile)
					if ncl, err := loadConfigs(); err == nil {
						log.Printf("[%s] updated configuration", k)
						cl[k] = ncl[k]
					}
				}
			}
		}
	}()

	for k, c := range cl {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			for _, entry := range cl[k].Entries {
				if strings.HasPrefix(r.URL.Path, entry.Path) {
					forward := r.URL.Path[len(entry.Path):]
					log.Printf("[%s] %s %s => %s%s", k, r.Method, r.URL.Path, entry.Backend, forward)
					r.URL.Path = forward
					r.Header.Set("X-Script-Name", forward)
					entry.proxy.ServeHTTP(w, r)
					return
				}
			}
			http.NotFound(w, r)
		})
		go func(mux *http.ServeMux, k string, c Config) {
			log.Printf("[%s] server %s started", k, c.Address)
			if err = http.ListenAndServe(c.Address, mux); err != nil {
				log.Fatal(err)
			}
		}(mux, k, c)
	}

	quit := make(chan bool)
	<-quit
}
