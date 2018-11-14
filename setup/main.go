package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/icrowley/fake"
)

var initialAdmin LocalUser
var allCommands SyncCommandSet
var currDir string
var binaryName string
var defaultCliVersion = "0.1.1"

var (
	tenant            = flag.StringP("tenant", "t", "", "Tenant name to create")
	adminEndpoint     = flag.StringP("admin-endpoint", "a", "https://7h1u0s6a44.execute-api.us-east-1.amazonaws.com/Prod", "Admin Endpoint. Default is QA")
	adminUser         = flag.String("admin-user", "admin", "Admin user for tenant")
	domain            = flag.StringP("domain", "d", "qabambe.com", "Tenant domain. Default is qabambe.com")
	operation         = flag.StringP("operation", "o", "", "Operation to conduct [setup|teardown]")
	numberUsers       = flag.IntP("users", "u", 100, "Number of users to create")
	numberSecrets     = flag.IntP("secrets", "s", 500, "Number of secrets to create")
	numberPermissions = flag.IntP("permissions", "p", 50, "Unique Permissions Per User")
	secretLength      = flag.IntP("secret-length", "l", 100, "Length of each secret data")
	cliVersion        = flag.StringP("cli-version", "v", "", "Version of Cli to use for setup/teardown")
)

func main() {
	flag.Parse()
	*tenant = strings.ToLower(*tenant)
	if *operation != "setup" && *operation != "teardown" {
		fmt.Printf("error: operation flag did not match op. Value: '%s'\n", *operation)
		flag.Usage()
		os.Exit(1)
	} else if *operation == "teardown" && *tenant == "" {
		fmt.Println("error: must specify tenant")
		flag.Usage()
		os.Exit(1)
	}

	currDir, _ = os.Getwd()
	ensureCliDownloaded()

	if *tenant == "" {
		*tenant = strings.Replace(strings.ToLower(fake.Company()), " ", "-", -1)
		fmt.Println("blank tenant name. generated random: " + *tenant)
	}

	status := 0
	if *operation == "setup" {
		// create tenant
		err := initTenant()
		if err != nil {
			fmt.Println("failed to create tenant")
			os.Exit(1)
		}
		// create data structures in memory for tenant
		initData()

		// create data structures on tenant
		err = DoSetup()
		if err != nil {
			fmt.Println(err)
			status = 1
		}
		os.Exit(status)
	} else if *operation == "teardown" {
		err := DoTeardown()
		if err != nil {
			fmt.Println(err)
			status = 1
		}
	}
	os.Exit(status)
}

func ensureCliDownloaded() {
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
				return
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
			os.Exit(1)
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
			os.Exit(1)
		}
		defer resp.Body.Close()
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			fmt.Printf("failed to create cli file: %v", err)
			os.Exit(1)
		}
		err = os.Chmod("thy", 0777)
		if err != nil {
			fmt.Printf("failed to set cli permissions to 0777: %v", err)
			os.Exit(1)
		}
	}
	// if here we should use cli in working directory
	if runtime.GOOS == "windows" {
		binaryName = ".\\" + binaryName
	} else {
		binaryName = "./" + binaryName
	}
}
