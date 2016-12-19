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

type EldoradoFeedParser struct {
	FeedReader
}

func (e EldoradoFeedParser) ParseFeed(ctx context.Context, feedFile multipart.File) {
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
			YML_NAMES_MAP, reflect.TypeOf(EldoradoOffer{}))
	}()
}

type EldoradoOffer struct {
	XMLName xml.Name `xml:"offer"`

	Id        string `xml:"id,attr"`
	Available string `xml:"available,attr"`
	Type      string `xml:"type,attr"`

	Uri         string `xml:"url"`
	Price       string `xml:"price"`
	CurrencyId  string `xml:"currencyId"`
	CategoryId  string `xml:"categoryId"`
	Picture     string `xml:"picture"`
	Vendor      string `xml:"vendor"`
	Model       string `xml:"model"`
	Description string `xml:"description"`
	Cpa         string `xml:"cpa"`
	Name        string `xml:"name"`

	Attributes []Attribute
}

func (o *EldoradoOffer) GetProductInfo() (interface{}, error) {
	uri := strings.Split(o.Uri, "?")[0]
	body, err := GetBody(uri)
	if err != nil {
		return nil, err
	}

	bodyReader := strings.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, err
	}

	name := doc.Find(".pp-description .text-b-o-c span").First().Text()
	description := doc.Find(".pp-description-text").First().Text()
	o.Description = description
	o.Name = name
	o.Uri = uri

	attrHandler := func(i int, s *goquery.Selection) {
		name := strings.TrimSuffix(s.Find("th div div").First().Text(), ":")
		if name == "" {
			return
		}
		value := s.Find("td").First().Text()
		attr := Attribute{Name: name, Value: value}
		o.Attributes = append(o.Attributes, attr)
	}

	doc.Find(".pp-characteristics-table").Find("tr").Each(attrHandler)

	return o, nil
}
