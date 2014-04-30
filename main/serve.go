package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/bdon/transit_timelines"
	"github.com/bdon/go.nextbus"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// takes as an argument a directory containing uncompressed GTFS files
// ttimeline feeds/ --compile : outputs stops/schedules based on all GTFS feeds.
//  serve these compiled files through NGINX.

func main() {
	s := transit_timelines.NewSystemState()
	ticker := time.NewTicker(10 * time.Second)
	cleanupTicker := time.NewTicker(60 * time.Second)
	mutex := sync.RWMutex{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		var time int

		if _, ok := r.Form["after"]; ok {
			time, _ = strconv.Atoi(r.Form["after"][0])
		}

		mutex.RLock()

		var result []byte
		if time > 0 {
			result, _ = json.Marshal(s.After(time))
		} else {
			result, _ = json.Marshal(s.Runs)
		}
		mutex.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprintf(w, string(result))
	}

	tick := func(unixtime int) {
		log.Println("Fetching from NextBus...")
		response := nextbus.Response{}
		get, err := http.Get("http://webservices.nextbus.com/service/publicXMLFeed?command=vehicleLocations&a=sf-muni&r=N&t=0")
		if err != nil {
			log.Println(err)
			return
		}
		defer get.Body.Close()
		str, _ := ioutil.ReadAll(get.Body)
		xml.Unmarshal(str, &response)

		mutex.Lock()
		s.AddResponse(response, unixtime)
		mutex.Unlock()
		log.Println("Done Fetching.")
	}

	go func() {
		for {
			select {
			case t := <-ticker.C:
				tick(int(t.Unix()))
			}
		}
	}()

	go func() {
		for {
			select {
			case t := <-cleanupTicker.C:
				log.Println("Deleting runs older than 12 hours.")
				mutex.Lock()
				s.DeleteOlderThan(60*60*12, int(t.Unix()))
				mutex.Unlock()
				log.Println("Done cleaning up.")
			}
		}
	}()

	// do the initial thing
	go tick(int(time.Now().Unix()))

	http.HandleFunc("/locations.json", handler)
	log.Println("Serving on port 8080.")
	http.ListenAndServe(":8080", nil)
}
