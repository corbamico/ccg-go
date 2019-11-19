// Copyright 2019-2020 ccg-go authors. All rights reserved.
// Use of this source code is governed by GNU GENERAL PUBLIC LICENSE version 3 that can be
// found in the LICENSE file.

package internal

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

//CCGRestService serv HTTP service
type CCGRestService struct {
	diamClient diamClient
}

//NewCCGRestService create a new Service
func NewCCGRestService(setting *Settings) CCGRestService {
	return CCGRestService{
		diamClient: newDiamClient(setting),
	}
}

//Run on CCGRestService
func (s *CCGRestService) Run() {
	//1. run diamClien on background
	go func() {
		s.diamClient.run()
	}()

	//2. arm "/ccr/slr/str" handler
	http.HandleFunc("/ccr", handler(s, messageGy))
	http.HandleFunc("/slr", handler(s, messageSySLR))
	http.HandleFunc("/str", handler(s, messageSySTR))

	//3. listen and Serve
	log.Printf("REST Server Serve at %s\n", configs.LocalRESTServerAddr)
	log.Fatal(http.ListenAndServe(configs.LocalRESTServerAddr, nil))
}

func handler(s *CCGRestService, msgType int) func(w http.ResponseWriter, r *http.Request) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.ContentLength == 0 || r.Method != "POST" {
			io.WriteString(w, "{\"error\":\"invalid request\"}")
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil || !isJSON(body) {
			io.WriteString(w, "{\"error\":\"invalid request\"}")
			return
		}
		io.WriteString(w, s.diamClient.sendJSON(body, msgType))
	}
	return h
}
