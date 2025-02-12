// TODO - error channel from aggregation of results from loader pods
// TODO
// TODO

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	flag "github.com/spf13/pflag"
	vegeta "github.com/tsenart/vegeta/lib"
)

const (
	cmdEndpointName = "command"
)

var (
	useIP     = flag.Bool("use-ip", false, "Use IP for aggregation")
	serveData = []byte{}
	lock      = sync.Mutex{}

	// for selecting pods during load
	selector = flag.String("selector", "run=vegeta", "The label selector for load runner pods")
)

type respWrapper struct {
	Data  interface{}
	Error string
}

func taskLoadtest(model *postLoaderModel) (status int, resp []byte) {
	log.Println("---Starting test task")
	results, err := runTest(model)
	var wrapper respWrapper
	if err != nil {
		fmt.Println("error running load test: " + err.Error())
		wrapper = respWrapper{
			Error: fmt.Sprintf("error running load test: %v", err),
		}
		status = 1
	} else {
		wrapper = respWrapper{
			Data: results,
		}
	}
	if !*redash {
		resp, _ = json.Marshal(&wrapper)
	} else {
		redashData := vegetaResultsToRedash(results)
		resp, _ = json.Marshal(redashData)
	}
	log.Println("---Finished test task")
	return status, resp
}

func runTest(model *postLoaderModel) ([]vegeta.Metrics, error) {
	// TODO : figure out why DNS resolution of pods isnt working
	*useIP = true
	var errAny error

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("Error creating config: %v", err)
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error client: %v", err)
		return nil, err
	}
	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{
		LabelSelector: *selector,
	})
	if err != nil {
		fmt.Printf("Error getting pods: %v", err)
		return nil, err
	}
	loadbots := []*corev1.Pod{}
	for ix := range pods.Items {
		pod := &pods.Items[ix]
		if pod.Status.PodIP == "" {
			continue
		}
		loadbots = append(loadbots, pod)
	}
	numberLoadBots := len(loadbots)
	parts := []vegeta.Metrics{}
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(numberLoadBots)

	// split rate among available loadbots
	fmt.Printf("Found %d loadbots for load test\n", len(loadbots))
	ratePer := int(float64(model.Rate) / math.Max(1.0, float64(numberLoadBots)))
	fmt.Printf("Spreading total rate %d rps to %d rps per bot\n", model.Rate, ratePer)
	model.Rate = ratePer

	bodyMarshalled, err := json.Marshal(model)
	clientTimeout := time.Duration(*loadDuration*6/5) * time.Second
	if err != nil {
		return parts, err
	}
	for ix := range loadbots {
		go func(ix int) {
			defer wg.Done()
			pod := loadbots[ix]
			var data []byte
			log.Printf("Sending job to loadbot %s\n", pod.Name)
			if *useIP {
				url := "http://" + pod.Status.PodIP + ":8080/command"

				req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyMarshalled))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				client.Timeout = clientTimeout
				resp, err := client.Do(req)
				if err != nil {
					fmt.Printf("Error posting task to loader: %v\n", err)
					if errAny == nil {
						errAny = err
					}
					return
				}
				defer resp.Body.Close()
				if data, err = ioutil.ReadAll(resp.Body); err != nil {
					fmt.Printf("Error reading load task result: %v\n", err)
					if errAny == nil {
						errAny = err
					}
					return
				}
			} else {
				var err error
				podPath := fmt.Sprintf("/api/v1/namespaces/default/pods/%s:8080/proxy/%s", pod.Name, cmdEndpointName)
				// NOT WORKING - not sure why doesnt resolve
				data, err = clientset.RESTClient().Post().AbsPath(podPath).Timeout(clientTimeout).Body(bodyMarshalled).DoRaw()
				if err != nil {
					fmt.Printf("Error proxying to pod %v: %v\n", podPath, err)
					return
				}
			}
			var metrics vegeta.Metrics
			if err := json.Unmarshal(data, &metrics); err != nil {
				fmt.Printf("Error decoding: %v\n", err)
				return
			}
			lock.Lock()
			defer lock.Unlock()
			parts = append(parts, metrics)
		}(ix)
	}
	wg.Wait()
	return parts, err
}
