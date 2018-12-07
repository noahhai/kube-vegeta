// TODO : make this concurrency safe when run in serve mode; spin a context off of the global flags
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/joncalhoun/qson"

	"github.com/icrowley/fake"
	flag "github.com/spf13/pflag"
)

var allCommands SyncCommandSet
var currDir string
var binaryName string
var foundCli bool
var defaultCliVersion = "0.1.1"

var (
	serve             = flag.Bool("serve", false, "Specifies if the app should run in server mode")
	redash            = flag.Bool("redash", false, "Specifies if the app should format the data for redash")
	port              = flag.Int("port", 3000, "Port to run on if --serve flag specified")
	tenant            = flag.StringP("tenant", "t", "", "Tenant name to create")
	adminEndpoint     = flag.StringP("admin-endpoint", "a", "https://7h1u0s6a44.execute-api.us-east-1.amazonaws.com/Prod", "Admin Endpoint. Default is QA")
	adminUser         = flag.String("admin-user", "admin", "Admin user for tenant")
	adminPassword     string
	domain            = flag.StringP("domain", "d", "qabambe.com", "Tenant domain. Default is qabambe.com")
	operation         = flag.StringP("operation", "o", "", "Operation to conduct [setup|teardown|test|full]")
	numberUsers       = flag.IntP("users", "u", 10, "Number of users to create")
	numberSecrets     = flag.IntP("secrets", "s", 50, "Number of secrets to create")
	numberPermissions = flag.IntP("permissions", "p", 5, "Unique Permissions Per User")
	secretLength      = flag.IntP("secret-length", "l", 100, "Length of each secret data")
	cliVersion        = flag.StringP("cli-version", "v", "", "Version of Cli to use for setup/teardown")
	loadDuration      = flag.Int("load-duration", 10, "duration of load test in seconds")
	loadRate          = flag.Int("load-rate", 10, "load test rps")
)

func main() {
	onStartup()
	if err := validateCmd(true); err != nil {
		failOnCli(err.Error())
	}

	if *serve {
		http.HandleFunc("/command", serveFunc)
		log.Printf("starting to serve on port %d\n", *port)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
		return
	}

	if err := ensureCliDownloaded(); err != nil {
		os.Exit(1)
	}
	preRun()
	status, res := runTasks()
	if res != nil && len(res) > 0 {
		fmt.Println("output:")
		fmt.Println(string(res))
	}
	os.Exit(status)
}

func onStartup() {
	flag.Parse()
	currDir, _ = os.Getwd()
}

func preRun() {
	// TODO : update this as we change it
	adminPassword = *adminUser + "@1" + *adminUser + "@1"
	*tenant = strings.ToLower(*tenant)
	if *tenant == "" {
		*tenant = strings.Replace(strings.ToLower(fake.Company()), " ", "-", -1)
		fmt.Println("blank tenant name. generated random: " + *tenant)
	}
}

func failOnCli(err string) error {
	fmt.Println(err)
	flag.Usage()
	os.Exit(1)
	return nil
}
func failOnServer(err string) error {
	fmt.Println(err)
	return errors.New(err)
}

func validateCmd(ignoreServe bool) error {
	var errMsg string
	if ignoreServe && *serve {
		return nil
	}
	if *operation != "setup" && *operation != "teardown" && *operation != "test" && *operation != "full" {
		errMsg = fmt.Sprintf("error: operation flag did not match a valid operation. Value: '%s'\n", *operation)

	} else if *operation == "teardown" && *tenant == "" {
		errMsg = "error: must specify tenant"
	}
	if *loadDuration > 3000 {
		errMsg = "error: --load-duration has max of 3000 seconds"
	}
	if errMsg != "" {
		if *serve {
			return failOnServer(errMsg)
		} else {
			return failOnCli(errMsg)
		}
	}
	return nil
}

func respFromError(err error) (status int, resp []byte, model *postLoaderModel) {
	m, _ := json.Marshal(err)
	return 1, m, nil
}

func taskSetup() (status int, resp []byte, testModel *postLoaderModel) {
	log.Println("---Starting setup task")
	err := createRemoteTenant()
	if err != nil {
		fmt.Println("failed to create tenant")
		return respFromError(err)
	}
	model := &postLoaderModel{
		Tenant:   *tenant,
		Domain:   *domain,
		Rate:     *loadRate,
		Duration: *loadDuration,
		// TODO : number workers
	}

	secretPaths := prepareDataLocally()
	model.SecretPaths = secretPaths

	tokens, err := populateRemoteTenant()
	if err != nil {
		fmt.Println(err)
		return respFromError(err)
	}
	if len(tokens) > 0 {
		model.Tokens = tokens
	}

	log.Println("---Finished setup task")
	resp, _ = json.Marshal(map[string]string{"tenant": *tenant})
	return 0, resp, model
}

func taskTeardown() error {
	log.Println("---Starting teardown task")
	err := DoTeardown()
	if err != nil {
		fmt.Println(err)
		return err
	}
	log.Println("---Finished teardown task")
	return nil
}

func runTasks() (status int, resp []byte) {
	status = 0
	resp = []byte{}
	var testModel *postLoaderModel
	doAll := *operation == "full"
	if doAll || *operation == "setup" {
		status, resp, testModel = taskSetup()
		if status != 0 {
			return status, resp
		}
		saveTestModel(testModel.Tenant, testModel)
	}
	if status == 0 && (doAll || *operation == "test") {
		if testModel == nil {
			testModel = getTestModel(*tenant)
		}
		if testModel == nil {
			return 1, []byte("failed to load test model for tenant: " + *tenant)
		}
		s, r := taskLoadtest(testModel)
		status |= s
		resp = r
	}
	if doAll || *operation == "teardown" {
		if err := taskTeardown(); err != nil {
			status = 1
			resp = []byte(err.Error())
		}
	}
	return status, resp
}

// TODO : implement these and we can run "test" as standalone command
func saveTestModel(tenant string, model *postLoaderModel) {

}
func getTestModel(tenant string) *postLoaderModel {
	return nil
}

func serveFunc(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.RequestURI)
	var params argsModel
	var err error
	if r.Method == "POST" {
		decoder := json.NewDecoder(r.Body)
		err = decoder.Decode(&params)
	} else {
		err = qson.Unmarshal(&params, r.URL.RawQuery)
	}

	if err != nil && err != io.EOF {
		log.Printf("Error assembling required prameters: %s\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error: " + err.Error()))
		return
	}

	params.Apply()
	if err := validateCmd(false); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error: " + err.Error()))
		return
	}
	if params.Tenant == "" {
		// fresh tenant every time unless specified
		*tenant = strings.Replace(strings.ToLower(fake.Company()), " ", "-", -1)
		fmt.Println("Updating tenant name to: " + *tenant)
	}
	preRun()

	// validation
	if *operation == "teardown" && *tenant == "" {
		log.Printf("must specify tenant name for teardown")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := ensureCliDownloaded(); err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Println("operation: " + *operation)
	status, resp := runTasks()
	if resp != nil && len(resp) > 0 {
		w.Write(resp)
	}
	if status == 1 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

}

func logAndError(err string) error {
	err = "Error: " + err
	fmt.Println(err)
	return errors.New(err)
}

func ensureCliDownloaded() error {
	if foundCli {
		return nil
	}
	cliVersionLocal := *cliVersion
	fmt.Println("checking for vault cli")

	if runtime.GOOS == "windows" {
		binaryName = "thy.exe"
	} else {
		binaryName = "thy"
	}

	if cliVersionLocal == "" {
		// check current directory

		if _, err := os.Stat(path.Join(currDir, binaryName)); os.IsNotExist(err) {
			log.Println("cli not found in current directory: " + path.Join(currDir, binaryName))
		} else if err != nil {
			log.Println("error checking for cli in " + path.Join(currDir, binaryName) + ". Error: " + err.Error())
		} else {
			log.Println("cli version not specified and found cli in current directory")
			foundCli = true
		}
		if !foundCli {
			// check in path
			_, err := exec.LookPath(binaryName)
			if err == nil {
				fmt.Println("cli version not specified and found cli in path")
				return nil
			} else {
				fmt.Println("cli not found in path")
			}
		}

	}
	if cliVersionLocal != "" || !foundCli {
		if cliVersionLocal == "" {
			cliVersionLocal = defaultCliVersion
		}
		fmt.Println("cli version specified or not found locally. downloading version: " + cliVersionLocal)
		execPath := path.Join(currDir, binaryName)
		out, err := os.Create(execPath)
		if err != nil {
			return logAndError(fmt.Sprintf("error creating file for cli: %v", err))
		}
		defer out.Close()
		var execSuffix, bitness, osName string
		osName = runtime.GOOS
		if runtime.GOARCH == "amd64" || false {
			bitness = "x64"
		} else {
			bitness = "x86"
		}
		if osName == "windows" {
			execSuffix = ".exe"
			osName = ""
		} else {
			osName = "-" + osName
		}

		cliUrl := fmt.Sprintf("https://cli.qabambe.com/cli/%s/thy-%s%s-%s%s", cliVersionLocal, cliVersionLocal, osName, bitness, execSuffix)
		fmt.Println("downloading cli from: " + cliUrl)
		resp, err := http.Get(cliUrl)
		if err != nil {
			return logAndError(fmt.Sprintf("failed to fetch cli at %s. Error: \n%v\n", cliUrl, err))
		}
		defer resp.Body.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return logAndError(fmt.Sprintf("failed to create cli file: %v", err))
		}
		err = os.Chmod(execPath, 0777)
		if err != nil {
			return logAndError(fmt.Sprintf("failed to set cli permissions to 0777: %v", err))
		}
	}

	// if here we should use cli in working directory
	if runtime.GOOS == "windows" {
		binaryName = ".\\" + binaryName
	} else {
		binaryName = "./" + binaryName
	}

	return nil
}

type argsModel struct {
	Tenant            string
	AdminEndpoint     string
	AdminUser         string
	AdminPassword     string
	Redash            *bool
	Domain            string
	Operation         string
	NumberUsers       int
	NumberSecrets     int
	NumberPermissions int
	SecretLength      int
	CliVersion        string
	LoadDuration      int
	LoadRate          int
}

func (a *argsModel) Apply() {
	if a.Redash != nil && !*a.Redash {
		*redash = false
	} else {
		*redash = true
	}
	if a.Tenant != "" {
		*tenant = a.Tenant
	}
	if a.AdminEndpoint != "" {
		*adminEndpoint = a.AdminEndpoint
	}
	if a.AdminPassword != "" {
		*adminEndpoint = a.AdminPassword
	}
	if a.Domain != "" {
		*domain = a.Domain
	}
	if a.Operation != "" {
		*operation = a.Operation
	}
	if a.NumberUsers > 0 {
		*numberUsers = a.NumberUsers
	}
	if a.NumberSecrets > 0 {
		*numberSecrets = a.NumberSecrets
	}
	if a.NumberPermissions > 0 {
		*numberPermissions = a.NumberPermissions
	}
	if a.SecretLength > 0 {
		*secretLength = a.SecretLength
	}
	if a.CliVersion != "" {
		*cliVersion = a.CliVersion
	}
	if a.LoadDuration > 0 {
		*loadDuration = a.LoadDuration
	}
	if a.LoadRate > 0 {
		*loadRate = a.LoadRate
	}
}
