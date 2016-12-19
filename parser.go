package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/context"

	// "github.com/franela/goreq"
	"github.com/franela/goreq"
	"github.com/golang/glog"
)

type Feed struct {
	file        multipart.File
	callbackURI string
	fileName    string
}

type ParserState struct {
	*sync.RWMutex
	stats map[string]int
}

func (p ParserState) GetStat(key string) int {
	p.RLock()
	defer p.RUnlock()
	if v, ok := p.stats[key]; ok {
		return v
	}
	return 0
}

func (p ParserState) SetStat(key string, delta int) {
	p.Lock()
	defer p.Unlock()
	if v, ok := p.stats[key]; ok {
		p.stats[key] = v + delta
	} else {
		p.stats[key] = delta
	}
}

func (p *ParserState) CleanStats() {
	p.Lock()
	defer p.Unlock()
	p.stats = map[string]int{}
}

type ProductExtractor interface {
	GetProductInfo() (interface{}, error)
}

func GetBody(uri string) (string, error) {
	proxy := GetProxy()
	defer ReleaseProxy(proxy)

	resp, err := goreq.Request{
		Uri:       uri,
		UserAgent: GetUserAgent(),
		Proxy:     proxy,
	}.Do()

	// request := gorequest.New().Get(uri).Set("UserAgent", GetUserAgent())
	// resp, body, errs := request.Timeout(10 * time.Second).Proxy(proxy).End()

	// if len(errs) != 0 {
	// 	for _, err := range errs {
	// 		glog.Errorln(err)
	// 	}
	// 	return "", errs[0]
	// }

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%v - %s", resp.StatusCode, uri)
	}

	// return body, nil
	return resp.Body.ToString()
}

type Scrapper struct {
	id                   int
	productExtractorChan <-chan ProductExtractor
	productChan          chan<- interface{}
	parserState          *ParserState
}

func (s Scrapper) Scrap(ctx context.Context) {
	s.parserState.SetStat("active-scrappers", 1)
	go func() {
		defer func() {
			glog.Infoln(fmt.Sprintf("Scrapper %d finished", s.id))
			s.parserState.SetStat("active-scrappers", -1)
		}()

		glog.Infoln(fmt.Sprintf("Scrapper %d started", s.id))
		for s.parserState.GetStat("reading-uri") == 1 || len(s.productExtractorChan) != 0 {
			select {
			case <-ctx.Done():
				return
			case productExtractor := <-s.productExtractorChan:
				productInfo, err := productExtractor.GetProductInfo()
				if err != nil {
					glog.Errorln(err)
					s.parserState.SetStat("scrapping-errors", 1)
				} else {
					s.productChan <- productInfo
					s.parserState.SetStat("scrapped-success", 1)
				}
			}
		}
	}()
}

type FeedParser interface {
	ParseFeed(context.Context, multipart.File)
}

type FeedWriter struct {
	waitGroup       *sync.WaitGroup
	productChan     <-chan interface{}
	startTokensChan chan xml.Token
	parserState     *ParserState
}

func (f FeedWriter) WriteFeed(ctx context.Context, fileName, callbackURI string) {
	f.waitGroup.Add(1)
	f.parserState.SetStat("writing-feed", 1)

	go func() {
		var tBuffer bytes.Buffer
		_, err := tBuffer.Write([]byte(xml.Header))
		if err != nil {
			panic(err)
		}

		enc := xml.NewEncoder(&tBuffer)
		enc.Indent("", "  ")

		defer func() {
			glog.Infoln("Feed writer finished")
			f.parserState.SetStat("writing-feed", -1)
			f.waitGroup.Done()
		}()

		for f.parserState.GetStat("active-scrappers") != 0 || len(f.productChan) != 0 {
			select {
			case <-ctx.Done():
				return
			case token := <-f.startTokensChan:
				if err := enc.EncodeToken(token); err != nil {
					glog.Errorln(err)
				}
			case product := <-f.productChan:
				if err := enc.Encode(product); err != nil {
					glog.Errorln(err)
				}
			default:
				continue
			}
		}

		enc.Flush()

		_, err = tBuffer.Write([]byte("</offers></shop></yml_catalog>"))
		if err != nil {
			glog.Errorln(err)
		}
		// glog.Infoln(tBuffer.String())
		if *writeToFile {
			writeFile(fileName, &tBuffer)
		} else {
			sendFile(fileName, callbackURI, &tBuffer)
		}
	}()
}

type FeedReader struct {
	waitGroup *sync.WaitGroup

	productExtractorChan chan ProductExtractor
	parserState          *ParserState
	startTokensChan      chan xml.Token
}

type Parser struct {
	ctx                 context.Context
	feedWriterWaitGroup *sync.WaitGroup
	feedParserWaitGroup *sync.WaitGroup

	scrappersPool  []Scrapper
	scrappersCount int

	feedReader           FeedReader
	feedWriter           FeedWriter
	productExtractorChan chan ProductExtractor
	readyParsersChan     chan *Parser
	state                *ParserState
	fileName             string
}

func (p *Parser) Init() {
	p.state = &ParserState{&sync.RWMutex{}, map[string]int{}}
	p.feedWriterWaitGroup = &sync.WaitGroup{}
	p.feedParserWaitGroup = &sync.WaitGroup{}
	p.productExtractorChan = make(chan ProductExtractor, 100)
	productChan := make(chan interface{}, 100)
	p.feedWriter = FeedWriter{
		waitGroup:       p.feedWriterWaitGroup,
		productChan:     productChan,
		parserState:     p.state,
		startTokensChan: make(chan xml.Token, 10),
	}

	p.feedReader = FeedReader{
		parserState:          p.state,
		waitGroup:            p.feedParserWaitGroup,
		productExtractorChan: p.productExtractorChan,
		startTokensChan:      p.feedWriter.startTokensChan,
	}

	for i := 0; i < p.scrappersCount; i++ {
		scrapper := Scrapper{
			parserState:          p.state,
			id:                   i + 1,
			productExtractorChan: p.productExtractorChan,
			productChan:          productChan,
		}
		p.scrappersPool = append(p.scrappersPool, scrapper)
	}
}

func (p *Parser) Start(ctx context.Context, f Feed) {
	p.fileName = f.fileName

	shopID := strings.Split(p.fileName, ".")[0]
	var feedParser FeedParser
	switch shopID {
	case "shopart":
		feedParser = ShopArtFeedParser{p.feedReader}
	case "eldorado":
		feedParser = EldoradoFeedParser{p.feedReader}
	case "go":
		feedParser = GoFeedParser{p.feedReader}
	case "fotos":
		feedParser = FotosFeedParser{p.feedReader}
	default:
		panic(fmt.Sprintf("No parser for file %s", p.fileName))
	}

	feedParser.ParseFeed(ctx, f.file)
	p.feedWriter.WriteFeed(ctx, f.fileName, f.callbackURI)

	for _, scrapper := range p.scrappersPool {
		scrapper.Scrap(ctx)
	}
	p.feedWriterWaitGroup.Wait()
	p.fileName = ""
	p.state.CleanStats()
	p.readyParsersChan <- p
}

func (p Parser) Stop() {
	p.feedParserWaitGroup.Wait()
	p.feedWriterWaitGroup.Wait()
}

type ParserOverseer struct {
	parsersCount     int
	scrappersCount   int
	feedC            chan Feed
	readyParsersChan chan *Parser
	parsersPool      []*Parser
}

func (po *ParserOverseer) Start(ctx context.Context) {
	po.readyParsersChan = make(chan *Parser, po.parsersCount)
	po.feedC = make(chan Feed, 100)
	for i := 0; i < po.parsersCount; i++ {
		p := Parser{
			scrappersCount:   po.scrappersCount,
			readyParsersChan: po.readyParsersChan,
		}
		p.Init()
		po.parsersPool = append(po.parsersPool, &p)
		po.readyParsersChan <- &p
	}
	po.Listen(ctx)
}

func (po ParserOverseer) WaitAndStop() {
	// We need to wait until all parser will gracefully finish
	glog.Infof("Waiting for parsers")
	for range po.parsersPool {
		<-po.readyParsersChan
	}
}

func (po ParserOverseer) Listen(ctx context.Context) {
	glog.Infoln("Listening for files...")
	go func() {
		defer glog.Infoln("Finishing parsing process...")
		for {
			select {
			case <-ctx.Done():
				return
			case file := <-po.feedC:
				p := <-po.readyParsersChan
				go (*p).Start(ctx, file)
			}
		}
	}()
}

type Stats map[string]map[string]int

func (po ParserOverseer) GetStats() (stats []Stats) {
	for _, p := range po.parsersPool {
		stats = append(stats, Stats{p.fileName: p.state.stats})
	}
	return
}

func sendFile(fileName, callbackURI string, file io.Reader) {
	var b bytes.Buffer

	writer := multipart.NewWriter(&b)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		panic(err)
	}
	io.Copy(part, file)
	writer.Close()

	_, err = goreq.Request{
		Method:      "POST",
		ContentType: writer.FormDataContentType(),
		Uri:         callbackURI,
		Body:        b.String(),
	}.Do()

	if err != nil {
		panic(err)
	}
}

func writeFile(fileName string, file *bytes.Buffer) {
	f, err := os.Create(fileName)
	if err != nil {
		glog.Errorln(err)
		return
	}
	defer f.Close()
	f.WriteString(file.String())
}
