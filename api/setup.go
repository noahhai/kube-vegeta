package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/icrowley/fake"
)

func HandleCommands(cmdPipe <-chan Command, errPipe chan<- error, resultPipe chan<- *CmdResult, wg *sync.WaitGroup) {

	for {
		c, ok := <-cmdPipe
		if !ok {
			return
		}
		if len(c.GetArgs()) == 0 {
			continue
		}
		cmdArgs := addConfigArg(c.GetArgs())
		cmd := exec.Command(binaryName, cmdArgs...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			errFull := fmt.Errorf("err executing cmd: %s %s\nErr: \n%s\nOutput:\n %s", binaryName, strings.Join(cmdArgs, " "), err, string(output))
			fmt.Println(errFull)
			errPipe <- errFull
			return
		} else {
			fmt.Printf(" " + strings.ToLower(c.GetType())[:1])
			if c.GetType() == "token" {
				resultPipe <- GetTokenResult(output)
			}
		}
		wg.Done()
	}
}

func populateRemoteTenant() (tokens []string, err error) {
	numWorkers := runtime.NumCPU()
	cmdPipe := make(chan Command, *numberUsers+*numberSecrets)
	defer close(cmdPipe)
	errPipe := make(chan error, numWorkers)
	finishPipe := make(chan bool)
	defer close(finishPipe)
	resultPipe := make(chan *CmdResult, *numberUsers)

	var cmdWait sync.WaitGroup

	var tokenWait sync.WaitGroup
	tokenWait.Add(*numberUsers)

	tokens = make([]string, 0, *numberUsers)
	// spawn token collector
	go func() {
		for result := range resultPipe {
			if result.Type != "token" {
				fmt.Printf("Warning: unhandled result type of type: %s, value: %s\n", result.Type, result.Value)
			} else {
				tokens = append(tokens, result.Value)
				tokenWait.Done()
			}
		}
	}()

	// spawn command worrkers
	fmt.Printf("Creating %d workers for setup\n", numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			HandleCommands(cmdPipe, errPipe, resultPipe, &cmdWait)
		}()
	}

	fmt.Println("Beginning operations/object creation (c=config,u=user,s=secret,p=permission): ")

	errored := false
	// TODO : optimize permisison creation by building config json locally and updating all at once
	for i, syncSetMember := range allCommands {
		fmt.Printf("running with %d procs\n", runtime.NumCPU())
		cmdWait.Add(len(syncSetMember))

		go func() {
			// wait for all commands to execute
			cmdWait.Wait()
			finishPipe <- true
		}()
		// enqueue all commands
		go func() {
			for _, asyncCommand := range syncSetMember {
				if errored {
					return
				}
				cmdPipe <- asyncCommand
			}
		}()
		select {
		case <-finishPipe:
			fmt.Println("")
			fmt.Printf("Finished stage %d\n", i)
			break
		case e := <-errPipe:
			errored = true
			fmt.Println("")
			fmt.Printf("Op cancelled at stage %d due to error\n: %v", i, e)
			close(resultPipe)
			return nil, e
		}
	}
	// signal last of tokens has been sent back and await aggregator to finish aggregating them
	fmt.Println("done with queuing all commands")
	close(resultPipe)
	tokenWait.Wait()

	if !errored {
		fmt.Println("Finished setup")
	}

	// return tokens so they can be used to auth with many users for a more-realistic load test
	return tokens, nil
}

func addConfigArg(args []string) []string {
	args = append(args, "--config")
	args = append(args, ".thy.yml")
	return args
}

type LocalUser struct {
	Name string
	Pass string
}

type Node struct {
	Parent   *Node
	Children []*Node
	Name     string
}

func NewNode(name string, parent *Node) *Node {
	return &Node{
		Name:     name,
		Children: []*Node{},
		Parent:   parent,
	}
}
func buildTreeRoot() *Node {
	root := Node{
		Children: []*Node{},
		Name:     "secrets",
	}
	numberChildren := *numberPermissions
	for i := 0; i < numberChildren; i++ {
		newNodeName := fake.IPv4()
		root.Children = append(root.Children, NewNode(newNodeName, &root))
	}
	return &root
}

func getNodePath(n *Node, delim string) string {
	path := n.Name
	for n.Parent != nil {
		path = n.Parent.Name + "/" + path
		n = n.Parent
	}
	return path
}

func getFirstLevelPaths(root *Node) (paths []string) {
	for _, c := range root.Children {
		paths = append(paths, c.Name)
	}
	return paths
}

func addNodeToTree(root, node *Node) {
	numberRootChildren := int32(len(root.Children))
	childIndex := rand.Int31n(numberRootChildren)
	currNode := root.Children[childIndex]
	// top-heavy random walk down tree
	for rand.Float32() < 0.50 && len(currNode.Children) > 0 {
		childIndex := rand.Int31n(numberRootChildren)
		currNode = root.Children[childIndex]
	}
	node.Parent = currNode
	currNode.Children = append(currNode.Children, node)
}

func prepareDataLocally() (secretPaths []string) {
	allCommands = SyncCommandSet{}

	// need to clear auth from last call
	allCommands = append(allCommands, []Command{
		&BaseCommand{
			Args: []string{"auth", "clear"},
		},
	})

	// update local config - do this rather than passing as flags for efficiency (cache auth token)
	// TODO : make concurrency safe
	allCommands = append(allCommands, []Command{
		&ConfigCommand{
			Path: "tenant",
			Val:  *tenant,
		},
	})
	allCommands = append(allCommands, []Command{
		&ConfigCommand{
			Path: "auth.username",
			Val:  *adminUser,
		},
	})
	allCommands = append(allCommands, []Command{
		&ConfigCommand{
			Path: "auth.password",
			Val:  adminPassword,
		},
	})
	allCommands = append(allCommands, []Command{
		&ConfigCommand{
			Path: "domain",
			Val:  *domain,
		},
	})

	userList := []string{}
	// creation of users / secrets can happen simultaneously
	userSecretCreateCommands := make(AsyncCommandSet, 0, *numberUsers+*numberSecrets)
	for i := 0; i < *numberUsers; i++ {
		name := fake.EmailAddress()
		userList = append(userList, name)
		//pass, _ := uuid.NewV4()
		pass := name + "@1"
		userSecretCreateCommands = append(userSecretCreateCommands, &UserCreateCommand{
			Name: name,
			//Pass: pass.String(),
			Pass: pass,
		})
	}

	secretPaths = []string{}
	secretTreeRoot := buildTreeRoot()
	for i := 0; i < *numberSecrets; i++ {
		secretName := fake.IPv4()
		secretNode := NewNode(secretName, nil)
		addNodeToTree(secretTreeRoot, secretNode)
		secretPath := getNodePath(secretNode, "/")
		secretPaths = append(secretPaths, secretPath)

		data := fake.CharactersN(*secretLength)

		userSecretCreateCommands = append(userSecretCreateCommands, &SecretCreateCommand{
			Path: secretPath,
			Data: data,
		})
	}
	allCommands = append(allCommands, userSecretCreateCommands)

	numberPermissions := *numberUsers * *numberPermissions
	permissionCreateCommands := make(AsyncCommandSet, 0, numberPermissions)
	rootFolders := getFirstLevelPaths(secretTreeRoot)
	for _, u := range userList {
		for _, p := range rootFolders {
			permissionCreateCommands = append(permissionCreateCommands, &PermissionCreateCommand{
				User: fmt.Sprintf("users:%s", u),
				Path: fmt.Sprintf("%s/<.*>", p),
			})
		}
	}
	allCommands = append(allCommands, permissionCreateCommands)

	numberTokens := *numberUsers
	tokenCreateCommands := make(AsyncCommandSet, 0, numberTokens)
	for _, u := range userList {
		tokenCreateCommands = append(tokenCreateCommands, &TokenCreateCommand{
			User: u,
		})
	}
	allCommands = append(allCommands, tokenCreateCommands)

	return secretPaths
}

func createRemoteTenant() error {
	// create tenant
	url := *adminEndpoint
	if !strings.HasSuffix(url, "/") {
		url = url + "/"
	}
	url = url + "tenant"
	fmt.Println("create request to: " + url + " for tenant: " + *tenant)

	body := map[string]interface{}{
		"tenant": *tenant,
		"user":   *adminUser,
	}

	asBytes, err := json.Marshal(body)
	if err != nil {
		fmt.Println("failed to marshal create tenant request body")
		return err
	}
	c := &http.Client{}

	//provision tenant
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(asBytes))
	if err != nil {
		fmt.Printf("Failed to create new request: %v\n", err)
		return err
	}
	req.Header.Set("Authorization", "Basic NDhkZjg0NzEtYjRiNi00ZWU1LTlkN2MtNGMzYWYxM2ZhZThhOmQxMzFkY2M4LTRkYWEtNDg4MS1iNzgwLTU2ZTZmOWY3ZTE3YQ==")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		fmt.Println("failed to post to tenant create endpoint: " + err.Error())
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		fmt.Println("Failed request")
		return errors.New("Failed to create tenant")
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read create request response")
		return err
	} else {
		fmt.Println("response: ", string(respBody))
	}

	//provision initial admin
	body = map[string]interface{}{
		"username": *adminUser,
		"password": adminPassword,
	}

	asBytes, err = json.Marshal(body)
	if err != nil {
		fmt.Println("failed to marshal initial admin request body")
		return err
	}
	uri := fmt.Sprintf("https://%s.%s/initialize", *tenant, *domain)
	req, err = http.NewRequest("POST", uri, bytes.NewBuffer(asBytes))
	if err != nil {
		fmt.Printf("Failed to create new request: %v\n", err)
		return err
	}
	resp, err = c.Do(req)
	if err != nil {
		fmt.Println("failed to post to initialize endpoint: " + err.Error())
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		fmt.Println("Failed request")
		return errors.New("Failed to create tenant")
	}
	defer resp.Body.Close()

	respBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read create initial admin response")
		return err
	} else {
		fmt.Println("response: ", string(respBody))
	}

	return nil
}
