// Copyright 2019-2020 ccg-go authors. All rights reserved.
// Use of this source code is governed by GNU GENERAL PUBLIC LICENSE version 3 that can be
// found in the LICENSE file.

package main

import (
	"log"

	"github.com/corbamico/ccg-go/internal"
)

func main() {
	var cfg internal.Settings
	var err error
	if cfg, err = internal.LoadSettings("config.json"); err != nil {
		log.Println(err)
		return
	}
	s := internal.NewCCGRestService(&cfg)
	s.Run()
}
