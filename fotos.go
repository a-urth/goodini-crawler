package main

import (
	"encoding/xml"
	"mime/multipart"
	"reflect"
	"strings"

	"golang.org/x/net/context"

	"github.com/PuerkitoBio/goquery"
	"github.com/golang/glog"
)

// from inner to outer node
var FOTOS_NAMES_MAP = map[string]string{
	"item":  "item",
	"items": "items",
	"main":  "catalog",
	"outer": "price",
}

type FotosFeedParser struct {
	FeedReader
}

func (e FotosFeedParser) ParseFeed(ctx context.Context, feedFile multipart.File) {
	e.waitGroup.Add(1)
	e.parserState.SetStat("reading-uri", 1)
	go func() {
		defer func() {
			glog.Infoln("Uri reader finished")
			e.waitGroup.Done()
			e.parserState.SetStat("reading-uri", -1)
			feedFile.Close()
		}()

		glog.Infoln("Uri reader started")
		XMLParse(ctx, feedFile, e.productExtractorChan, e.startTokensChan,
			FOTOS_NAMES_MAP, reflect.TypeOf(FotosOffer{}))
	}()
}

type Image struct {
	XMLName xml.Name `xml:"image"`
	URI     string   `xml:",chardata"`
}

type FotosOffer struct {
	XMLName xml.Name `xml:"item"`

	Id        string `xml:"id,attr"`
	Available string `xml:"available,attr"`
	Bid       string `xml:"bid,attr"`

	Name           string `xml:"name"`
	Uri            string `xml:"url"`
	Images         []Image
	Price          string `xml:"priceuah"`
	CategoryId     string `xml:"categoryId"`
	Vendor         string `xml:"vendor"`
	Description    string `xml:"description"`
	AvailableField string `xml:"available"`

	Attributes []Attribute
}

func (o *FotosOffer) GetProductInfo() (interface{}, error) {
	o.Uri = strings.Split(o.Uri, "?")[0]
	body, err := GetBody(o.Uri)
	if err != nil {
		return nil, err
	}
	bodyReader := strings.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, err
	}

	attributeHandler := func(i int, s *goquery.Selection) {
		name := s.Find("td.name").First().Text()
		value := s.Find("td.value").First().Text()
		glog.Infoln(name, value)
		if len(value) >= 200 {
			return
		}
		o.Attributes = append(o.Attributes, Attribute{Name: name, Value: value})
	}
	doc.Find(".clear.properties.tab_div table tr.full.short").Each(attributeHandler)

	if o.AvailableField == "Склад" {
		o.Available = "true"
	} else {
		o.Available = "false"
	}
	o.AvailableField = ""

	return o, nil
}
