// Copyright 2019-2020 ccg-go authors. All rights reserved.
// Use of this source code is governed by GNU GENERAL PUBLIC LICENSE version 3 that can be
// found in the LICENSE file.

package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"
)

//Encoding for json2diam
type Encoding struct{}

//JSON2DiamEncoding support avpcode:vendorid and avpname format
var JSON2DiamEncoding = Encoding{}

//Encode []byte into diam.Message
func (enc *Encoding) Encode(dst *diam.Message, src []byte) (err error) {
	var js map[string]interface{}
	var avps *diam.AVP
	//1. check src is valid json
	if err = json.Unmarshal(src, &js); err != nil {
		return err
	}
	//2. range every key/value
	for key, value := range js {
		if avps, err = encode(key, value, dst.Header.ApplicationID); err != nil {
			return err
		}
		if avps.Code == avp.SessionID && avps.VendorID == 0 {
			//2.1 put it as first AVP if it is SessionID avp
			dst.InsertAVP(avps)
		} else {
			//2.2 add avp into diam.Message
			dst.AddAVP(avps)
		}
	}
	return nil
}

//Decode  diam.Message into []byte
func (enc *Encoding) Decode(src *diam.Message) (dst []byte, err error) {
	var b bytes.Buffer
	jsonGroupData(&b, src.AVP, src.Header.ApplicationID)
	return b.Bytes(), nil
}

func jsonAVP(b *bytes.Buffer, avp *diam.AVP, appid uint32) {
	//1. print avp name
	if dictAVP, err := dict.Default.FindAVPWithVendor(appid, avp.Code, avp.VendorID); err != nil {
		//1.1 print as "20230:2011" if no avp_name found,
		fmt.Fprintf(b, "\"%d:%d\": ", avp.Code, avp.VendorID)
	} else {
		//1.2 print as "avp_name" if avp_name found
		fmt.Fprintf(b, "\"%s\": ", dictAVP.Name)
	}

	//2. print avp value
	if avp.Data.Type() != diam.GroupedAVPType {
		//2.1 print avp value if nest avp
		fmt.Fprint(b, jsonData(avp.Data))
	} else {
		//2.2 print group avp
		jsonGroupData(b, avp.Data.(*diam.GroupedAVP).AVP, appid)
	}

}
func jsonGroupData(b *bytes.Buffer, avps []*diam.AVP, appid uint32) {
	fmt.Fprintln(b, "{")
	for index, avp := range avps {
		//1. print each line "avp_name:avp_value"
		jsonAVP(b, avp, appid)
		//2. print "," for each line if not last line
		if index != len(avps)-1 {
			fmt.Fprintln(b, ",")
		}
	}
	fmt.Fprintln(b, "}")
	return
}
func jsonData(data datatype.Type) string {
	switch data.Type() {
	case datatype.UnknownType:
		return "null"
	case datatype.AddressType:
		return fmt.Sprintf("\"%s\"", net.IP(data.(datatype.Address)))
	case datatype.DiameterIdentityType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.DiameterIdentity)))
	case datatype.DiameterURIType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.DiameterURI)))
	case datatype.EnumeratedType:
		return fmt.Sprintf("%d", data.(datatype.Enumerated))
	case datatype.Float32Type:
		return fmt.Sprintf("%f", data.(datatype.Float32))
	case datatype.Float64Type:
		return fmt.Sprintf("%f", data.(datatype.Float64))
	case datatype.GroupedType:
		return "\"Grouped\""
	case datatype.IPFilterRuleType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.IPFilterRule)))
	case datatype.IPv4Type:
		return fmt.Sprintf("\"%s\"", net.IP(data.(datatype.IPv4)))
	case datatype.Integer32Type:
		return fmt.Sprintf("%d", data.(datatype.Integer32))
	case datatype.Integer64Type:
		return fmt.Sprintf("%d", data.(datatype.Integer64))
	case datatype.OctetStringType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.OctetString)))
	case datatype.UTF8StringType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.UTF8String)))
	case datatype.QoSFilterRuleType:
		return fmt.Sprintf("\"%s\"", string(data.(datatype.QoSFilterRule)))
	case datatype.TimeType:
		return fmt.Sprintf("\"%s\"", time.Time(data.(datatype.Time)).Format("2006-01-02T15:04:05-0700"))
	case datatype.Unsigned32Type:
		return fmt.Sprintf("%d", data.(datatype.Unsigned32))
	case datatype.Unsigned64Type:
		return fmt.Sprintf("%d", data.(datatype.Unsigned64))
	default:
		return "\"\""
	}
}
func jsonEscape(s string) string {
	var b []byte
	var err error
	if b, err = json.Marshal(s); err != nil {
		return ""
	}
	return string(b)
}
func isJSON(s []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(s, &js) == nil
}

func encode(key string, value interface{}, appid uint32) (avp *diam.AVP, err error) {
	var avpcode int
	var subAVP *diam.AVP
	//var dictAVP *dict.AVP
	var matched bool
	vendoridInt := int(0)
	dictAVP := &dict.AVP{Code: 0, VendorID: 0}

	//1. handle key
	if matched, _ = regexp.MatchString(`^[0-9]+$`, key); matched {
		//1.1. handle key as "avp_code"
		if avpcode, err = strconv.Atoi(key); err != nil {
			return nil, err
		}
		dictAVP.VendorID = 0
		dictAVP.Code = uint32(avpcode)

	} else if matched, _ = regexp.MatchString(`^[0-9]+:[0-9]+$`, key); matched {
		//1.2. handle key as "avp_code:avp_vendorid"
		avpNumbers := strings.Split(key, ":")
		if avpcode, err = strconv.Atoi(avpNumbers[0]); err != nil {
			return nil, err
		}
		if vendoridInt, err = strconv.Atoi(avpNumbers[1]); err != nil {
			return nil, err
		}
		dictAVP.VendorID = uint32(vendoridInt)
		dictAVP.Code = uint32(avpcode)

	} else {
		//1.3. handle key as "avp_name"
		if dictAVP, err = dict.Default.FindAVP(appid, key); err != nil {
			return nil, err
		}
	}

	//2. handle value
	//TODO, should be handle value according dictAVP found in dictionary
	switch value.(type) {
	case float64:
		return diam.NewAVP(dictAVP.Code, 0, dictAVP.VendorID, datatype.Unsigned32(value.(float64))), nil
	case string:
		return diam.NewAVP(dictAVP.Code, 0, dictAVP.VendorID, datatype.UTF8String(value.(string))), nil
	case map[string]interface{}:
		grouped := &diam.GroupedAVP{
			AVP: make([]*diam.AVP, 0),
		}
		for key, value := range value.(map[string]interface{}) {
			if subAVP, err = encode(key, value, appid); err != nil {
				return nil, err
			}
			grouped.AddAVP(subAVP)
		}
		return diam.NewAVP(dictAVP.Code, 0, dictAVP.VendorID, grouped), nil
	default:
		return nil, errors.New("Unknown Data Type")
	}
}
