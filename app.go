package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"time"

	cron "github.com/robfig/cron/v3"

	"github.com/julienschmidt/httprouter"
)

type Cron struct {
	listenErrCh chan error
	crons       []Job
}

var (
	cronTask          *cron.Cron
	reloadCronTask    *cron.Cron
	heartbeatCronTask *cron.Cron

	httpClient *http.Client
	cronM      *Cron
)

type Job struct {
	Interval string
	Handler  func()
}

type API struct {
}

func main() {

	webserver := initConfigAndModules()

	cronM = &Cron{
		listenErrCh: make(chan error),
	}

	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    1 * time.Second,
		DisableCompression: true,
	}
	httpClient = &http.Client{Transport: tr}

	RegisterHeartBeatCron()
	webserver.Run()
}

func RegisterHeartBeatCron() {
	c := &Cron{
		listenErrCh: make(chan error),
	}

	c.register(Job{
		Interval: "* * * * *", //every minute
		Handler: func() {
			Println(nil, "alive")

			res, err := httpClient.Get("https://tokopedia-rca-app.herokuapp.com/")
			if err != nil {
				return
			}

			defer res.Body.Close()

			data, _ := ioutil.ReadAll(res.Body)
			Println(nil, string(data))
		},
	})

	heartbeatCronTask = cron.New()
	c.Run(heartbeatCronTask)
}

func initConfigAndModules() *WebServer {
	cfgWeb := &Option{
		Environment: "development",
		Domain:      "",
		Port:        ":" + os.Getenv("PORT"),
	}

	webserver := NewWeb(cfgWeb)
	webserver.RegisterAPI(
		API{},
	)

	return webserver
}

func (api API) Register(router *httprouter.Router) {

	router.GET("/",
		api.HandleCommand,
	)

}

func (api API) HandleCommand(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	WriteResponse(w, "heartbeat")
}

func WriteResponse(w http.ResponseWriter, response interface{}) {
	b, _ := json.Marshal(response)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(200)
	w.Write(b)
}

func CaptureCronPanic(handler func()) func() {
	return func() {
		defer func() {
			var err error
			r := recover()
			if r != nil {
				switch t := r.(type) {
				case string:
					err = errors.New(t)
				case error:
					err = t
				default:
					err = errors.New("unknown error")
				}
				Println(nil, "[Cron Process Got Panic] ", err.Error())
				panic(fmt.Sprintf("%s, Stack: %+v", "Cron Process Got Panic!!!", err.Error()))
			}
		}()
		handler()
	}
}

func (c *Cron) register(j Job) {
	j.Handler = CaptureCronPanic(j.Handler)
	c.crons = append(c.crons, j)
}

func (c *Cron) ListenError() <-chan error {
	return c.listenErrCh
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (c *Cron) Run(taskCron *cron.Cron) {

	if len(c.crons) == 0 {
		Println(nil, "[!!!] Cron is Empty!")
		return
	}

	for _, j := range c.crons {

		//fnameTemp := strings.Split(getFunctionName(j.Handler), "/")
		//fname := fnameTemp[len(fnameTemp)-1]
		_, err := taskCron.AddFunc(j.Interval, j.Handler)
		if err != nil {
			Println(nil, "Assign Job ERROR: ", err)
		}
	}

	taskCron.Start()
}
