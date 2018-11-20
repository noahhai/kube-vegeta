// TODO - error channel from aggregation of results from loader pods
// TODO
// TODO

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	flag "github.com/spf13/pflag"
	vegeta "github.com/tsenart/vegeta/lib"
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

func taskLoadtest() (status int, resp []byte) {
	log.Println("---Starting test task")
	results, err := runTest()
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
	resp, _ = json.Marshal(&wrapper)
	log.Println("---Finished test task")
	return status, resp
}

func runTest() ([]vegeta.Metrics, error) {

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
	parts := []vegeta.Metrics{}
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(len(loadbots))
	for ix := range loadbots {
		go func(ix int) {
			defer wg.Done()
			pod := loadbots[ix]
			var data []byte
			if *useIP {
				url := "http://" + pod.Status.PodIP + ":8080/"
				resp, err := http.Get(url)
				if err != nil {
					fmt.Printf("Error getting: %v\n", err)
					return
				}
				defer resp.Body.Close()
				if data, err = ioutil.ReadAll(resp.Body); err != nil {
					fmt.Printf("Error reading: %v\n", err)
					return
				}
			} else {
				var err error
				data, err = clientset.RESTClient().Get().AbsPath("/api/v1/namespaces/default/pods/" + pod.Name + ":8080/proxy/").DoRaw()
				if err != nil {
					fmt.Printf("Error proxying to pod: %v\n", err)
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
	return parts, nil
}
