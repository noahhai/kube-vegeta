// TODO : make this concurrency safe when run in serve mode; spin a context off of the global flags
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NebulousLabs/fastrand"
	flag "github.com/spf13/pflag"

	vegeta "github.com/tsenart/vegeta/lib"
)

var (
	serve             = flag.Bool("serve", false, "Specifies if the app should run in server mode")
	port              = flag.Int("port", 3000, "Port to run on if --serve flag specified")
	reportPort        = flag.Int("report-port", 3001, "Port to run reporting on if --serve NOT specified")
	tenant            = flag.String("tenant", "", "Tenant name to create")
	domain            = flag.String("domain", "qabambe.com", "Tenant domain. Default is qabambe.com")
	secretPaths       []string
	secretPathsString = flag.String("secret-paths", "", "A comma separated list of secret paths to test")
	tokens            []string
	tokensString      = flag.String("tokens", "", "A comma separated list of valid auth tokens")
	rate              = flag.Int("rate", 1, "The QPS to send")
	duration          = flag.Duration("duration", 10*time.Second, "The duration of the load test")
	workers           = flag.Int("workers", 10, "The number of workers to use")
	staticTargeter    = flag.Bool("static-targeter", false, "Use static targeter rather than dynamic targeter")
)

// HTTPReporter outputs metrics over HTTP
type HTTPReporter struct {
	sync.Mutex
	metrics *vegeta.Metrics
}

func (h *HTTPReporter) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	metrics := h.GetMetrics()

	res.WriteHeader(http.StatusOK)
	reporter := vegeta.NewJSONReporter(metrics)
	reporter.Report(res)
}

// GetMetrics returns the current metrics for this reporter
func (h *HTTPReporter) GetMetrics() *vegeta.Metrics {
	h.Lock()
	defer h.Unlock()
	return h.metrics
}

// SetMetrics sets the current metrics for this reporter
func (h *HTTPReporter) SetMetrics(metrics *vegeta.Metrics) {
	h.Lock()
	defer h.Unlock()
	h.metrics = metrics
}

func main() {
	flag.Parse()
	validateCmd()

	if *serve {
		http.HandleFunc("/command", serveFunc)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
	} else {
		reporter := &HTTPReporter{}
		go func() {
			log.Println()
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *reportPort), reporter))
		}()
		metrics := doAttack()
		reporter.SetMetrics(metrics)
		log.Println("press any key to stop serving results and quit")
		reader := bufio.NewReader(os.Stdin)
		reader.ReadString('\n')
	}
}

func doAttack() *vegeta.Metrics {
	fmt.Println("preparing targeting")
	requestBase := fmt.Sprintf("https://%s.%s/secrets", *tenant, *domain)
	var targets []vegeta.Target
	if len(secretPaths) < 1 {
		secretPaths = strings.Split(*secretPathsString, ",")
	}

	// TODO : test perf between static and json attacker
	// tradeoff is that if we use static, we have to pre-select auth-path pairs
	// but with JSON targeter, we can use a generator for a new random pair each time
	var targeter vegeta.Targeter
	if !*staticTargeter {
		targetReader := NewTargetReader(requestBase, secretPaths, tokens)
		targeter = vegeta.NewJSONTargeter(targetReader, nil, nil)
	} else {
		for _, path := range secretPaths {
			path = strings.TrimPrefix(path, "/")
			targets = append(targets, vegeta.Target{
				Method: "GET",
				URL:    fmt.Sprintf("%s/%s", requestBase, path),
			})
			//fmt.Printf(fmt.Sprintf("adding target:%s/%s\n", requestBase, path))
		}
		targeter = vegeta.NewStaticTargeter(targets...)
	}

	log.Println("starting attack session")
	attacker := vegeta.NewAttacker(vegeta.Workers(uint64(*workers)))
	attackRate := vegeta.Rate{
		Freq: *rate,
		Per:  time.Second,
	}
	metrics := &vegeta.Metrics{}
	for res := range attacker.Attack(targeter, attackRate, *duration, "main") {
		metrics.Add(res)
		if res.Error != "" {
			fmt.Println(res.Error)
		}
	}
	log.Println("completed attack session")
	metrics.Close()
	return metrics
}

func serveFunc(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var params argsModel
	err := decoder.Decode(&params)
	secretPaths = params.SecretPaths
	tokens = params.Tokens
	if err == nil || err == io.EOF {
		err = params.Validate()
	}
	if err != nil {
		log.Printf("Error assembling required prameters: %s\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error: " + err.Error()))
		return
	}
	params.Apply()
	metrics := doAttack()
	if asBytes, err := json.Marshal(metrics); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error marshalling metrics for response: " + err.Error()))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(asBytes)
	}
}

type targetGenerator struct {
	root      string
	paths     []string
	pathsLen  int
	tokens    []string
	tokensLen int
	data      []byte
	readIndex int64
}

func NewTargetReader(root string, paths []string, tokens []string) io.Reader {
	return &targetGenerator{
		root:      root,
		paths:     paths,
		pathsLen:  len(paths),
		tokens:    tokens,
		tokensLen: len(tokens),
	}
}

func (t *targetGenerator) zeroReadIndex() {
	if t.readIndex <= 0 {
		return
	}
	t.data = t.data[t.readIndex:]
	t.readIndex = 0
}

// TODO : can we optimize this? each target is around 600 bytes and
// the attacker reads 4000 at a time...so need about 10 targets per invocation
func (t *targetGenerator) pushTargetBuffer() {
	t.zeroReadIndex()
	path := strings.TrimPrefix(t.paths[fastrand.Intn(t.pathsLen)], "/")
	header := http.Header{}
	token := t.tokens[fastrand.Intn(t.tokensLen)]
	header.Add("Authorization", token)
	// fmt.Printf("generated target of %s and header %s...\n", path, token[:10])
	target := vegeta.Target{
		Method: "GET",
		URL:    fmt.Sprintf("%s/%s", t.root, path),
		Header: header,
	}
	m, _ := json.Marshal(target)
	m = append(m, '\n')
	t.data = append(t.data, m...)
}

// Read reads forever since it is a generator; no EOF needed
// the JsonTargeter looks for '\n' as an object delimiter
func (t *targetGenerator) Read(p []byte) (n int, err error) {
	lenOut := len(p)
	lenOut64 := int64(lenOut)
	for lenOut64 > int64(len(t.data))-t.readIndex {
		t.pushTargetBuffer()
	}
	copy(p, t.data[t.readIndex:])
	t.readIndex += lenOut64
	return lenOut, nil
}

type argsModel struct {
	Tenant         string
	Domain         string
	Rate           int
	Duration       int
	SecretPaths    []string
	Tokens         []string
	StaticTargeter bool
	Workers        int
}

func validateCmd() {
	if *serve {
		return
	}
	missing := ""
	if *tenant == "" {
		missing = "--tenant"
	} else if *secretPathsString == "" {
		missing = "--secret-paths"
	} else if *tokensString == "" {
		missing = "--tokens"
	}

	if missing != "" {
		fmt.Printf("error: missing required flag: '%s'\n", missing)
		flag.Usage()
		os.Exit(1)
	}
}

func (a *argsModel) Validate() error {
	if a.Tenant == "" {
		return errors.New("must specify tenant")
	}
	if len(a.SecretPaths) == 0 {
		return errors.New("no secret paths specified")
	}
	if len(a.Tokens) == 0 {
		return errors.New("no auth tokens specified")
	}
	return nil
}

func (a *argsModel) Apply() {
	if a.Tenant != "" {
		*tenant = a.Tenant
	}
	if a.Domain != "" {
		*domain = a.Domain
	}
	if a.Duration > 0 {
		*duration = time.Duration(a.Duration) * time.Second
	}
	if a.Rate > 0 {
		*rate = a.Rate
	}
	if a.Workers > 0 {
		*workers = a.Workers
	}
	if a.StaticTargeter {
		*staticTargeter = a.StaticTargeter
	}
}
