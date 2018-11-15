// TODO : make this concurrency safe when run in serve mode; spin a context off of the global flags
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/icrowley/fake"
	flag "github.com/spf13/pflag"
)

var initialAdmin LocalUser
var allCommands SyncCommandSet
var currDir string
var binaryName string
var defaultCliVersion = "0.1.1"

var (
	serve             = flag.Bool("serve", false, "Specifies if the app should run in server mode")
	port              = flag.Int("port", 3000, "Port to run on if --serve flag specified")
	tenant            = flag.StringP("tenant", "t", "", "Tenant name to create")
	adminEndpoint     = flag.StringP("admin-endpoint", "a", "https://7h1u0s6a44.execute-api.us-east-1.amazonaws.com/Prod", "Admin Endpoint. Default is QA")
	adminUser         = flag.String("admin-user", "admin", "Admin user for tenant")
	adminPassword     string
	domain            = flag.StringP("domain", "d", "qabambe.com", "Tenant domain. Default is qabambe.com")
	operation         = flag.StringP("operation", "o", "", "Operation to conduct [setup|teardown|test|full]")
	numberUsers       = flag.IntP("users", "u", 100, "Number of users to create")
	numberSecrets     = flag.IntP("secrets", "s", 500, "Number of secrets to create")
	numberPermissions = flag.IntP("permissions", "p", 50, "Unique Permissions Per User")
	secretLength      = flag.IntP("secret-length", "l", 100, "Length of each secret data")
	cliVersion        = flag.StringP("cli-version", "v", "", "Version of Cli to use for setup/teardown")
)

func main() {
	onStartup()
	validateCmd()

	if *serve {
		http.HandleFunc("/command", serveFunc)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
	}

	if !ensureCliDownloaded() {
		os.Exit(1)
	}
	preRun()
	os.Exit(runTasks())
}

func onStartup() {
	flag.Parse()
	currDir, _ = os.Getwd()
}

func preRun() {
	// TODO : update this as we change it
	adminPassword = *adminUser + "@1"
	*tenant = strings.ToLower(*tenant)
	if *tenant == "" {
		*tenant = strings.Replace(strings.ToLower(fake.Company()), " ", "-", -1)
		fmt.Println("blank tenant name. generated random: " + *tenant)
	}
}

func validateCmd() {
	if *serve {
		return
	}
	if *operation != "setup" && *operation != "teardown" && *operation != "test" && *operation != "full" {
		fmt.Printf("error: operation flag did not match op. Value: '%s'\n", *operation)
		flag.Usage()
		os.Exit(1)
	} else if *operation == "teardown" && *tenant == "" {
		fmt.Println("error: must specify tenant")
		flag.Usage()
		os.Exit(1)
	}
}

func taskSetup() (status int) {
	log.Println("---Starting setup task")
	err := createRemoteTenant()
	if err != nil {
		fmt.Println("failed to create tenant")
		return 1
	}

	prepareDataLocally()

	err = populateRemoteTenant()
	if err != nil {
		fmt.Println(err)
		return 1
	}
	log.Println("---Finished setup task")
	return 0
}

func taskTeardown() (status int) {
	log.Println("---Starting teardown task")
	err := DoTeardown()
	if err != nil {
		fmt.Println(err)
		return 1
	}
	log.Println("---Finished teardown task")
	return 0
}

func taskLoadtest() (status int) {
	log.Println("---Starting test task")
	// todo
	log.Println("---Finished test task")
	return 0
}

func runTasks() (status int) {
	status = 0
	doAll := *operation == "full"
	if doAll || *operation == "setup" {
		status |= taskSetup()
	}
	if doAll || *operation == "test" {
		status |= taskLoadtest()
	}
	if doAll || *operation == "teardown" {
		status |= taskTeardown()
	}
	return status
}

func serveFunc(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var params argsModel
	if err := decoder.Decode(&params); err != nil {
		log.Printf("Failed to decode params: %v\n", err)
		w.Write([]byte("Error: " + err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		params.Apply()
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
		if !ensureCliDownloaded() {
			log.Printf("error getting cli")
			w.Write([]byte("error getting cli"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	status := runTasks()
	if status == 1 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func ensureCliDownloaded() bool {
	fmt.Println("checking for vault cli")

	if runtime.GOOS == "windows" {
		binaryName = "thy.exe"
	} else {
		binaryName = "thy"
	}

	foundCli := false
	if *cliVersion == "" {
		// check current directory
		if _, err := os.Stat(path.Join(currDir, binaryName)); os.IsNotExist(err) {
			fmt.Println("cli not found in current directory")
		} else {
			fmt.Println("cli version not specified and found cli in current directory")
			foundCli = true
		}
		if !foundCli {
			// check in path
			_, err := exec.LookPath(binaryName)
			if err == nil {
				fmt.Println("cli version not specified and found cli in path")
				return false
			} else {
				fmt.Println("cli not found in path")
			}
		}

	}
	if *cliVersion != "" || !foundCli {
		if *cliVersion == "" {
			*cliVersion = defaultCliVersion
		}
		fmt.Println("cli version specified or not found locally. downloading version: " + *cliVersion)
		execPath := path.Join(currDir, binaryName)
		out, err := os.Create(execPath)
		if err != nil {
			fmt.Printf("error creating file for cli: %v", err)
			return false
		}
		defer out.Close()
		var execSuffix, bitness, osName string
		osName = runtime.GOOS
		if runtime.GOARCH == "amd64" {
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
		cliUrl := fmt.Sprintf("https://cli.qabambe.com/cli/%s/thy-%s%s-%s%s", *cliVersion, *cliVersion, osName, bitness, execSuffix)
		resp, err := http.Get(cliUrl)
		if err != nil {
			fmt.Printf("failed to fetch cli at %s. Error: \n%v\n", cliUrl, err)
			return false
		}
		defer resp.Body.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Printf("failed to create cli file: %v", err)
			return false
		}
		err = os.Chmod("thy", 0777)
		if err != nil {
			fmt.Printf("failed to set cli permissions to 0777: %v", err)
			return false
		}
	}
	// if here we should use cli in working directory
	if runtime.GOOS == "windows" {
		binaryName = ".\\" + binaryName
	} else {
		binaryName = "./" + binaryName
	}
	return true
}

type argsModel struct {
	Tenant            string
	AdminEndpoint     string
	AdminUser         string
	AdminPassword     string
	Domain            string
	Operation         string
	NumberUsers       int
	NumberSecrets     int
	NumberPermissions int
	SecretLength      int
	CliVersion        string
}

func (a *argsModel) Apply() {
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
}
