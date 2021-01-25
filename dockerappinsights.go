package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jasonlvhit/gocron"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
)

const (
	azureMetadataUrl string = "http://169.254.169.254/metadata/instance/compute/tags/?api-version=2020-10-01&format=text"
	printCMDLog      bool   = true
)

var (
	telemetryClient    appinsights.TelemetryClient
	instrumentationKey string
	telInterval        uint64
	ctx                context.Context
	cli                *client.Client
)

type TelemetryData struct {
	Sessions    int
	LocationKey string
	ServerIP    string
}

type AzureMetadata struct {
	LocationKey string
	PublicIP    string
}

var logger *log.Logger

func InitLog() {
	logFile, err := os.OpenFile("./appinsights.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Panic("Open log file failed")
	}
	logger = log.New(logFile, "", log.Ldate|log.Ltime|log.Lshortfile)
}

var isLocalDebug *bool

func InitArg() {
	flag.StringVar(&instrumentationKey, "ik", "553da460-2f49-4fcc-bd75-9d9a122ddfc1", "set instrumentation key from azure portal") //test: 145e628d-5197-439a-9ccf-5efa2bbe42b5
	flag.Uint64Var(&telInterval, "ti", 10, "app insights telemetry event send interval")

	isLocalDebug = flag.Bool("debug", false, "is running in debug mode? By default is false")
	flag.Parse()

	_isLocalDebug := *isLocalDebug
	fmt.Println(_isLocalDebug)
}

func InitAzureEnv() AzureMetadata {
	amd := AzureMetadata{}
	if *isLocalDebug {

	} else {
		amd = GetPublicIPRequest(azureMetadataUrl, http.MethodGet, []byte(""), 0)

		if amd.PublicIP == "" || amd.LocationKey == "" {
			commonSimpleOutput(fmt.Sprint("GetPublicIPRequest-Cannot get public ip\n"))
			handleSimpleFail(fmt.Sprint("GetPublicIPRequest-Cannot get public ip\n"))
		}
	}

	return amd
}

func GetPublicIPRequest(url string, method string, postData []byte, timeOut time.Duration) AzureMetadata {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(postData))
	req.Header.Set("Metadata", "true")

	client := &http.Client{}
	if timeOut != 0 {
		client.Timeout = timeOut
	}
	resp, err := client.Do(req)
	if err != nil {
		handleSimpleFail(fmt.Sprintf("GetPublicIPRequest %s Error: %s\n", url, err.Error()))
	}
	defer resp.Body.Close()

	//commonSimpleOutput(fmt.Sprintf("GetPublicIPRequest %s response Status: %s, Headers: %s \n", url, resp.Status, resp.Header))
	body, _ := ioutil.ReadAll(resp.Body)
	//commonSimpleOutput(fmt.Sprintf("GetPublicIPRequest %s response Body: %s\n", url, string(body)))

	if string(body) != "" {
		res1 := strings.Split(string(body), ";")
		publicIP := ""
		locationKey := ""
		for _, res := range res1 {
			if strings.HasPrefix(res, "pip:") {
				publicIP = strings.Replace(res, "pip:", "", -1)
			}
			if strings.HasPrefix(res, "locationkey:") {
				locationKey = strings.Replace(res, "locationkey:", "", -1)
			}
		}

		return AzureMetadata{PublicIP: publicIP, LocationKey: locationKey}
	}
	return AzureMetadata{}
}

func taskWithParams(locationKey string, publicIP string) {
	//Check all running containers
	//----------------------------------
	allContainers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		handleSimpleFail(fmt.Sprint(err))
		return
	}

	vol := len(allContainers)

	data := &TelemetryData{
		Sessions:    vol,
		LocationKey: locationKey,
		ServerIP:    publicIP,
	}

	trace := appinsights.NewTraceTelemetry(fmt.Sprint(jsonMarshal(data)), appinsights.Information)

	// You can set custom properties on traces
	trace.Properties["locationKey"] = locationKey
	trace.Properties["serverIP"] = publicIP

	// You can also fudge the timestamp:
	trace.Timestamp = time.Now()

	// Finally, track it
	telemetryClient.Track(trace)
}

func main() {
	InitArg()

	InitLog()

	ctx = context.Background()
	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		handleSimpleFail(fmt.Sprint(err))
		return
	}

	telemetryClient = appinsights.NewTelemetryClient(instrumentationKey)
	/*turn on diagnostics to help troubleshoot problems with telemetry submission. */
	appinsights.NewDiagnosticsMessageListener(func(msg string) error {
		log.Printf("[%s] %s\n", time.Now().Format(time.UnixDate), msg)
		return nil
	})

	am := AzureMetadata{}

	if *isLocalDebug == false {
		am = InitAzureEnv()
	}

	gocron.Every(telInterval).Seconds().From(gocron.NextTick()).Do(taskWithParams, am.LocationKey, am.PublicIP)

	// Start all the pending jobs
	<-gocron.Start()
}

func jsonMarshal(i interface{}) string {
	s, _ := json.Marshal(i)
	return string(s)
}

func handleSimpleFail(msg string) {
	commonSimpleOutput(msg)
	return
	//logger.Panic(msg)
}

func commonSimpleOutput(msg string) {
	if printCMDLog {
		fmt.Print(msg)
	}
	logger.Print(msg)
}
