package main

import (
	"encoding/xml"
	"io"
	"reflect"

	"github.com/golang/glog"
	"golang.org/x/net/context"
)

type Attribute struct {
	XMLName xml.Name `xml:"param"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:",chardata"`
}

var YML_NAMES_MAP = map[string]string{
	"items": "offers",
	"item":  "offer",
	"main":  "shop",
	"outer": "yml_catalog",
}

func XMLParse(ctx context.Context, feed io.Reader,
	scrapperC chan<- ProductExtractor, startChan chan<- xml.Token,
	namesMap map[string]string, t reflect.Type) {
	decoder := xml.NewDecoder(feed)
	for i := 0; i != *urlLimit; {
		select {
		case <-ctx.Done():
			return
		default:
			token, err := decoder.Token()
			if err != nil {
				glog.Errorln(err)
				return
			}
			switch element := token.(type) {
			case xml.StartElement:
				if element.Name.Local == namesMap["item"] {
					v := reflect.New(t).Interface()
					err := decoder.DecodeElement(&v, &element)
					if err != nil {
						glog.Errorln(err)
					} else {
						i++
						scrapperC <- v.(ProductExtractor)
					}
				} else {
					startChan <- xml.CopyToken(token)
				}
			case xml.CharData:
				if element[0] != byte(10) {
					startChan <- xml.CopyToken(token)
				}
			case xml.EndElement:
				itsItems := element.Name.Local == namesMap["items"]
				itsMain := element.Name.Local == namesMap["main"]
				itsOuter := element.Name.Local == namesMap["outer"]
				if !(itsItems || itsMain || itsOuter) {
					startChan <- xml.CopyToken(token)
				}
			}
		}
	}
}
