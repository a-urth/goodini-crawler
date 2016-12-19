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

type GoFeedParser struct {
	FeedReader
}

func (e GoFeedParser) ParseFeed(ctx context.Context, feedFile multipart.File) {
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
			YML_NAMES_MAP, reflect.TypeOf(GoOffer{}))
	}()
}

type GoOffer struct {
	XMLName xml.Name `xml:"offer"`

	Id        string `xml:"id,attr"`
	Available string `xml:"available,attr"`
	Bid       string `xml:"bid,attr"`

	Uri         string `xml:"url"`
	Price       string `xml:"price"`
	CurrencyId  string `xml:"currencyId"`
	CategoryId  string `xml:"categoryId"`
	Picture     string `xml:"picture"`
	Store       string `xml:"store"`
	Pickup      string `xml:"pickup"`
	Delivery    string `xml:"delivery"`
	Name        string `xml:"name"`
	Description string `xml:"description"`

	Attributes []Attribute
}

func (o *GoOffer) GetProductInfo() (interface{}, error) {
	body, err := GetBody(o.Uri)
	if err != nil {
		return nil, err
	}
	bodyReader := strings.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, err
	}

	description := doc.Find(".product-description__item .text").First().Text()
	o.Description = description

	attributeHandler := func(i int, s *goquery.Selection) {
		name := s.Find(".properties-table__title").First().Text()
		value := s.Find(".properties-table__td").Last().Text()
		if len(value) >= 200 {
			return
		}
		o.Attributes = append(o.Attributes, Attribute{Name: name, Value: value})
	}

	doc.Find(".properties-table tr").Each(attributeHandler)
	return o, nil
}
