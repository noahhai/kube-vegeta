package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	vegeta "github.com/tsenart/vegeta/lib"
)

var (
	addr     = flag.String("address", "localhost:8080", "The address to serve on")
	selector = flag.String("selector", "", "The label selector for pods")
	useIP    = flag.Bool("use-ip", false, "Use IP for aggregation")
	sleep    = flag.Duration("sleep", 5*time.Second, "The sleep period between aggregations")

	serveData = []byte{}
	lock      = sync.Mutex{}
)

func getData() []byte {
	lock.Lock()
	defer lock.Unlock()
	return serveData
}

func setData(data []byte) {
	lock.Lock()
	defer lock.Unlock()
	serveData = data
}

func serveHTTP(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Access-Control-Allow-Origin", "*")
	res.WriteHeader(http.StatusOK)
	res.Write(getData())
}

func main() {
	flag.Parse()

	http.HandleFunc("/", serveHTTP)
	go http.ListenAndServe(*addr, nil)

	for {
		start := time.Now()
		loadData()
		latency := time.Now().Sub(start)
		if latency < *sleep {
			time.Sleep(*sleep - latency)
		}
		fmt.Printf("%v\n", time.Now().Sub(start))
	}
}

func getField(obj map[string]interface{}, fields ...string) (interface{}, bool) {
	nextObj, found := obj[fields[0]]
	if !found {
		return nil, false
	}
	if len(fields) > 1 {
		return getField(nextObj.(map[string]interface{}), fields[1:]...)
	}
	return nextObj, true
}
func loadData() error {

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("Error creating config: %v", err)
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error client: %v", err)
		return err
	}
	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{
		LabelSelector: *selector,
	})
	if err != nil {
		fmt.Printf("Error getting pods: %v", err)
		return err
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
	data, err := json.Marshal(parts)
	if err != nil {
		fmt.Printf("Error marshaling: %v", err)
	}
	setData(data)
	fmt.Printf("Updated.\n")
	return nil
}

func discoverPodsForLabel(label string) []corev1.Pod {
	return nil
}
