package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"github.com/tylerb/graceful"
)

type Set map[string]struct{}

var (
	po ParserOverseer

	host           = flag.String("host", "localhost:8001", "host address")
	proxyFile      = flag.String("proxyFile", "proxy.txt", "file with proxies")
	scrappersCount = flag.Int("scrappers", 100, "count of concurrent scrappers per parser")
	urlLimit       = flag.Int("url_limit", -1, "specify to limit the number of processed xml rows")
	parsersCount   = flag.Int("parsers", 1, "count of concurrent parsers")
	writeToFile    = flag.Bool("file", false, "flush result to file instead of sending to portal")

	availableParsers = Set{
		"shopart":  struct{}{},
		"eldorado": struct{}{},
		"go":       struct{}{},
		"fotos":    struct{}{},
	}
)

func addParseJob(c *echo.Context) error {
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		glog.Errorln(err)
		return c.String(http.StatusBadRequest, err.Error())
	}

	callback := c.Form("callbackUri")
	fileName := header.Filename

	shopID := strings.Split(fileName, ".")[0]
	if _, ok := availableParsers[shopID]; !ok {
		msg := fmt.Sprintf("There is no parser for file - %s", fileName)
		return c.String(http.StatusBadRequest, msg)
	}

	glog.Infoln(fmt.Sprintf("File: %s, callback: %s", fileName, callback))
	for _, p := range po.parsersPool {
		if p.fileName == fileName {
			msg := fmt.Sprintf("File - %s already in use", fileName)
			return c.String(http.StatusBadRequest, msg)
		}
	}
	po.feedC <- Feed{file, callback, fileName}
	return nil
}

func stats(c *echo.Context) error {
	return c.JSON(http.StatusOK, po.GetStats())
}

func main() {
	flag.Parse()

	LoadProxies(*proxyFile)

	ctx, cancelFunc := context.WithCancel(context.Background())
	po = ParserOverseer{
		parsersCount:   *parsersCount,
		scrappersCount: *scrappersCount,
	}
	po.Start(ctx)

	e := echo.New()
	e.SetDebug(true)

	e.Use(mw.Logger())
	e.Use(mw.Recover())

	e.Get("/stats", stats)
	e.Post("/parse", addParseJob)

	glog.Errorln(graceful.ListenAndServe(e.Server(*host), 5*time.Second))
	cancelFunc()
	po.WaitAndStop()
}
