package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/gorilla/mux"
	"github.com/yl2chen/cidranger"
)

// custom structure that conforms to RangerEntry interface
type customRangerEntry struct {
	ipNet   net.IPNet
	country string
}

// get function for network
func (b *customRangerEntry) Network() net.IPNet {
	return b.ipNet
}

// get function for network converted to string
func (b *customRangerEntry) NetworkStr() string {
	return b.ipNet.String()
}

// get function for ASN
func (b *customRangerEntry) Country() string {
	return b.country
}

// create customRangerEntry object using net and asn
func newCustomRangerEntry(ipNet net.IPNet, country string) cidranger.RangerEntry {
	return &customRangerEntry{
		ipNet:   ipNet,
		country: country,
	}
}

type API struct {
	Ranger cidranger.Ranger
}

type Items struct {
	Match  []Match `json:"matches"`
	Status string  `json:"status"`
}

type Match struct {
	Country string `json:"country"`
	Net     string `json:"network"`
}

// entry point
func main() {

	// Clone the given repository to the given directory
	const Path = "./cidr"
	_, err := git.PlainClone(Path, false, &git.CloneOptions{
		URL:      "https://github.com/herrbischoff/country-ip-blocks.git",
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println(err)
	}

	r, err := git.PlainOpen(Path)
	if err != nil {
		fmt.Println(err)
	}

	// Get the working directory for the repository
	w, err := r.Worktree()
	if err != nil {
		fmt.Println(err)
	}

	// Pull the latest changes from the origin remote and merge into the current branch
	err = w.Pull(&git.PullOptions{RemoteName: "origin"})

	if err != nil {
		fmt.Println(err)
	}

	api := &API{}

	// instantiate NewPCTrieRanger
	api.Ranger = cidranger.NewPCTrieRanger()

	err = filepath.Walk("./cidr/ipv4", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)

			for scanner.Scan() {
				_, network, _ := net.ParseCIDR(scanner.Text())
				api.Ranger.Insert(newCustomRangerEntry(*network, strings.TrimRight(filepath.Base(path), filepath.Ext(path))))
			}

			if err := scanner.Err(); err != nil {
				log.Fatal(err)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}

	router := mux.NewRouter()
	router.HandleFunc("/ip/{ip}", api.Get).Methods("GET")

	http.Handle("/", router)
	srv := &http.Server{
		Addr:        ":1234",
		IdleTimeout: 5 * time.Second,
		Handler:     router,
	}

	srv.ListenAndServe()
}

func (Api *API) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ip := vars["ip"]
	entries, err := Api.Ranger.ContainingNetworks(net.ParseIP(ip))
	if err != nil {
		fmt.Println("Api.Ranger.Contains()", err.Error())
		os.Exit(1)
	}
	var result Items
	for _, e := range entries {
		entry, ok := e.(*customRangerEntry)
		if !ok {
			continue
		}
		n := entry.NetworkStr()
		a := entry.Country()

		result.Status = "200"
		result.Match = append(result.Match, Match{Country: a, Net: n})

		outgoingJSON, error := json.Marshal(result)

		if error != nil {
			http.Error(w, error.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, string(outgoingJSON))
		return
	}
}
