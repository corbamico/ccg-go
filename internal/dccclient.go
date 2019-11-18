// Copyright 2019-2020 ccg-go authors. All rights reserved.
// Use of this source code is governed by GNU GENERAL PUBLIC LICENSE version 3 that can be
// found in the LICENSE file.

package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"time"

	"github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"
	"github.com/fiorix/go-diameter/v4/diam/sm"
	"github.com/fiorix/go-diameter/v4/diam/sm/smpeer"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

//Settings for this program
type Settings struct {
	OriginHost          string
	OriginRealm         string
	DiameterServerAddr  string
	LocalRESTServerAddr string
	ExtraDiameterXML    string
	DumpMessage         bool
	DumpPCAP            bool
	DumpFile            string
}

type diamClient struct {
	cfg           *sm.Settings
	sm            *sm.StateMachine
	client        *sm.Client
	channel       *channels
	conn          diam.Conn
	serverAddress string
	pcapFile      *os.File
	pcapWriter    *pcapgo.Writer
}
type message struct {
	json []byte
}
type channels struct {
	tx   chan message
	rx   chan message
	pcap chan message
}

var configs Settings

//LoadSettings from config.json
func LoadSettings(file string) (Settings, error) {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		return configs, err
	}
	jsonParser := json.NewDecoder(configFile)
	if err = jsonParser.Decode(&configs); err != nil {
		return configs, nil
	}
	if configs.ExtraDiameterXML != "" {
		err = dict.Default.LoadFile(configs.ExtraDiameterXML)
	}
	return configs, nil
}

func newDiamClient(setting *Settings) diamClient {
	cfg := &sm.Settings{
		OriginHost:    datatype.DiameterIdentity(setting.OriginHost),
		OriginRealm:   datatype.DiameterIdentity(setting.OriginRealm),
		VendorID:      2011,
		ProductName:   "ccg-go",
		OriginStateID: datatype.Unsigned32(time.Now().Unix()),
	}
	mux := sm.New(cfg)
	cli := &sm.Client{
		Handler:            mux,
		MaxRetransmits:     3,
		RetransmitInterval: time.Second,
		EnableWatchdog:     true,
		WatchdogInterval:   5 * time.Second,
		AuthApplicationID: []*diam.AVP{
			//RFC 4006, Credit Control
			diam.NewAVP(avp.AuthApplicationID, avp.Mbit, 0, datatype.Unsigned32(4)),
		},
	}
	return diamClient{
		cfg:    cfg,
		sm:     mux,
		client: cli,
		channel: &channels{
			tx:   make(chan message, 1000),
			rx:   make(chan message, 1000),
			pcap: make(chan message, 1000),
		},
		serverAddress: setting.DiameterServerAddr,
	}
}

func (c *diamClient) cleanup() {
	close(c.channel.tx)
	close(c.channel.rx)
	close(c.channel.pcap)
	if c.pcapFile != nil {
		c.pcapFile.Sync()
		c.pcapFile.Close()
	}
}

func (c *diamClient) run() {
	var err error
	//var conn diam.Conn

	//step 1. arm "CCA" handler
	c.sm.HandleFunc("CCA", func(con diam.Conn, m *diam.Message) {
		//1. dump in console
		if configs.DumpMessage {
			log.Printf("Recieve CCA from %s\n%s", con.RemoteAddr(), m)
		}
		//2. dump in pcap file
		if c.pcapWriter != nil {
			if bytes, err := m.Serialize(); err == nil {
				c.channel.pcap <- message{bytes}
			}
		}
		//3. return json to sendCCR (that wait on chan rx)
		json, _ := JSON2DiamEncoding.Decode(m)
		c.channel.rx <- message{json}
	})

	//step 2. connect to diameter server
	if c.conn, err = c.client.DialNetwork("tcp", c.serverAddress); err != nil {
		log.Fatalf("Client connect to server failed(%s).\n", err)
	}
	log.Printf("Client connect to server(%s) sucess.\n", c.serverAddress)

	//step 3. arm ctrl-c handler
	ctrl := make(chan os.Signal, 1)
	signal.Notify(ctrl, os.Interrupt, os.Kill)
	go signalHandler(ctrl, c)

	//step 4. create background pcap writing go-rountine
	go pcapWriteHandler(c)

	//step 5. recieving message from chan forever
	for {
		select {
		case <-c.conn.(diam.CloseNotifier).CloseNotify():
			c.cleanup()
			log.Fatalln("Client disconnected.")
			return
		case msg, ok := <-c.channel.tx:
			if ok {
				c.sendCCR(msg.json)
			}
		}
	}
}

func (c *diamClient) sendJSON(json []byte) (res string) {
	c.channel.tx <- message{json}
	select {
	case response, ok := <-c.channel.rx:
		if ok {
			res = string(response.json)
		}
	case <-time.After(2 * time.Second):
		res = `{"error":"timeout for wating CCA"}`
	}
	return
}
func (c *diamClient) sendCCR(json []byte) {
	//CommandCode,272,Credit Control;ApplicationId=4, Diameter Credit Control Application
	m := diam.NewRequest(272, 4, nil)
	meta, _ := smpeer.FromContext(c.conn.Context())
	sid := fmt.Sprintf("%s;%d;%d", configs.OriginHost, time.Now().Unix(), rand.Uint32())
	m.NewAVP(avp.OriginHost, avp.Mbit, 0, c.cfg.OriginHost)
	m.NewAVP(avp.OriginRealm, avp.Mbit, 0, c.cfg.OriginRealm)
	m.NewAVP(avp.DestinationHost, avp.Mbit, 0, meta.OriginHost)
	m.NewAVP(avp.DestinationRealm, avp.Mbit, 0, meta.OriginRealm)
	m.NewAVP(avp.AuthApplicationID, avp.Mbit, 0, datatype.Unsigned32(4))

	if err := JSON2DiamEncoding.Encode(m, json); err != nil {
		res := fmt.Sprintf(`{"error":%s}`, jsonEscape((err.Error())))
		c.channel.rx <- message{[]byte(res)}
		return
	}

	//Insert SessionID AVP, if not present in POST JSON
	if sessionid, _ := m.FindAVP(avp.SessionID, 0); sessionid == nil {
		m.InsertAVP(diam.NewAVP(avp.SessionID, avp.Mbit, 0, datatype.UTF8String(sid)))
	}

	//write in console
	if configs.DumpMessage {
		log.Printf("Sending CCR to %s\n%s", c.conn.RemoteAddr(), m)
	}

	//write pcap file if configured
	if c.pcapWriter != nil {
		if bytes, err := m.Serialize(); err == nil {
			c.channel.pcap <- message{bytes}
		}
	}
	m.WriteTo(c.conn)
}

func signalHandler(ctrl chan os.Signal, c *diamClient) {
	<-ctrl
	c.cleanup()
	log.Println("Client cleanup and exit.")
	os.Exit(0)
}

var dltUSER15 uint8 = 162

func pcapWriteHandler(c *diamClient) {
	var err error
	ticker := time.NewTicker(10 * time.Second)

	if !configs.DumpPCAP || configs.DumpFile == "" {
		return
	}

	//1. create pcap file
	if c.pcapFile, err = os.Create(configs.DumpFile); err != nil {
		log.Printf("Client create pcap file failed (%s).\n", err.Error())
		return
	}
	log.Printf("Client create pcap file(%s) success.\n", configs.DumpFile)
	c.pcapWriter = pcapgo.NewWriter(c.pcapFile)
	c.pcapWriter.WriteFileHeader(66536, layers.LinkType(dltUSER15))
	c.pcapFile.Sync()

	//2. recieve message from pcap chan
	for {
		select {
		case pcap, ok := <-c.channel.pcap:
			if c.pcapWriter != nil && ok {
				c.pcapWriter.WritePacket(gopacket.CaptureInfo{
					Timestamp:      time.Now(),
					CaptureLength:  len(pcap.json),
					Length:         len(pcap.json),
					InterfaceIndex: 0,
				}, pcap.json)
			}
		case <-ticker.C:
			if c.pcapFile != nil {
				c.pcapFile.Sync()
			}
		}
	}
}
