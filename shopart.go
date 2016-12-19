package main

import (
	"encoding/xml"
	"fmt"
	"mime/multipart"
	"reflect"
	"strings"

	"golang.org/x/net/context"

	"github.com/PuerkitoBio/goquery"
	"github.com/golang/glog"
)

type ShopArtFeedParser struct {
	FeedReader
}

func (e ShopArtFeedParser) ParseFeed(ctx context.Context, feedFile multipart.File) {
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
			YML_NAMES_MAP, reflect.TypeOf(ShopArtOffer{}))
	}()
}

type ShopArtOffer struct {
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
}

func (o ShopArtOffer) GetProductInfo() (interface{}, error) {
	body, err := GetBody(o.Uri)
	if err != nil {
		return nil, err
	}
	bodyReader := strings.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, err
	}

	name := doc.Find(".product-info .product_name").First().Text()
	if name == "" {
		return nil, fmt.Errorf("No info %s", o.Uri)
	}

	return o, nil
}
