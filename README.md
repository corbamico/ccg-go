# ccg-go

ccg-go works as REST HTTP server, recive json message from POST <http://ip:port/ccr>,  
and send it to Diameter Server.

## How to Use

* Just run ccg-go  
* Use *REST Client* plugin in *Visual Studio Code*  
  Sample REST json message as file *test/test-case.http*  
* REST format for AVP  
  ccg-go support two type formats:  

  ```json
  //format as "avp_code:vendor_id"
  {"20302:2011":"86139"}
  //format as "avp_name"
  {"calling-vlr-number":"86139"}
  ```

* co-works with wireshark  
  ccg-go write pcap as DLT_USER15 protocal,  
  You can config in wireshark as Edit->Preference->Protocal->DLT_USER, add  
  "USER 15(DLT=162)" as Payload 'diameter'

## Configuration

```json
    "originHost":          "1.client.ccg-go",
    "originRealm":         "client.ccg-go",
    "diameterServerAddr":  "10.253.191.56:16553", //remote Diameter Server IP/Port
    "localRESTServerAddr": ":8080",               //local address for REST server
    "extraDiameterXML":    "vendor.xml",          //extra xml for diameter dictionary
    "dumpMessage":         false,                 //print detail CCR/CCA in console?
    "dumpPCAP":            false,                 //dump packet to pcap file?
    "dumpFile":            "ccg-go.pcap"          //file name for wireshark
```

## Log

## Todo List

* [x] handle CCR/CCA diameter message
* [x] Dump detail CCR/CCA in console
* [x] Return as json format
* [x] Return detail CCA message as json
* [x] Write .pacap for wireshark
* [x] Generate SessionID for automaticlly

## License

Copyright (c) Corbamico  
GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007
