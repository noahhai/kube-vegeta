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
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/gobuffalo/uuid"
	"github.com/icrowley/fake"
)

func HandleCommands(cmdPipe <-chan Command, errPipe chan interface{}, resultPipe chan<- CmdResult, wg *sync.WaitGroup) {
	for {
		select {
		// case err := <-errPipe:
		// 	// forward to next worker
		// 	errPipe <- err
		// 	return
		case c := <-cmdPipe:
			cmdArgs := addConfigArg(c.GetArgs())
			cmd := exec.Command(binaryName, cmdArgs...)
			output, err := cmd.CombinedOutput()

			if err != nil {
				errFull := fmt.Errorf("Err executing cmd: thy %s\nErr: %s\nOutput: %s\n%s", strings.Join(cmdArgs, " "), err, string(output))
				errPipe <- errFull
			} else {
				fmt.Printf(" " + strings.ToLower(c.GetType())[:1])
				// TODO : if we need output for future actions
				// _ = output
				//resultPipe <- resultFromCmd(c.ResourceType, c.C)
			}
			wg.Done()
		}
	}
}

func fieldFromCmd(cmd, field string) string {
	r := regexp.MustCompile(field + `\s+(\S+)`)
	res := r.FindStringSubmatch(cmd)
	if len(res) > 0 {
		return res[0]
	}
	return ""
}

func resultFromCmd(t, cmd string) CmdResult {
	if t == "user" {
		res := CmdResult{
			Type:   t,
			Fields: map[string]string{},
		}
		res.Fields["name"] = fieldFromCmd(cmd, "name")
		res.Fields["pass"] = fieldFromCmd(cmd, "pass")
		return res
	}
	return CmdResult{}
}

func DoSetup() error {

	cmdPipe := make(chan Command)
	defer close(cmdPipe)
	errPipe := make(chan interface{})
	finishPipe := make(chan bool)
	defer close(finishPipe)
	resultPipe := make(chan CmdResult)
	defer close(resultPipe)

	numWorkers := runtime.NumCPU()
	_ = numWorkers
	var wg sync.WaitGroup
	fmt.Printf("Creating %d workers for setup\n", numWorkers)
	for i := 0; i < numWorkers; i++ {
		go HandleCommands(cmdPipe, errPipe, resultPipe, &wg)
	}

	fmt.Println("Beginning operations/object creation (c=config,u=user,s=secret,p=permission): ")

	// TODO : optimize permisison creation by building config json locally and updating all at once
	for i, syncSetMember := range allCommands {

		fmt.Printf("Beginning stage %d; %d operations\n", i, len(syncSetMember))
		wg.Add(len(syncSetMember))

		go func() {
			wg.Wait()
			finishPipe <- true
		}()
		for _, asyncCommand := range syncSetMember {
			cmdPipe <- asyncCommand
		}
		select {
		case <-finishPipe:
			fmt.Println("")
			fmt.Printf("Finished stage %d\n", i)
		case err := <-errPipe:
			close(cmdPipe)
			fmt.Println("")
			fmt.Printf("Op cancelled at stage %d due to error\n: %v", i, err)
			close(errPipe)
			break
		}
	}

	return nil
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

func initData() {
	allCommands = SyncCommandSet{}

	// update local config - do this rather than passing as flags for efficiency (cache auth token)
	localConfigCommands := []Command{
		&ConfigCommand{
			Path: "tenant",
			Val:  *tenant,
		},
		&ConfigCommand{
			Path: "auth.username",
			Val:  *adminUser,
		},
		&ConfigCommand{
			Path: "auth.password",
			Val:  *adminEndpoint + "@1",
		},
		&ConfigCommand{
			Path: "domain",
			Val:  *domain,
		},
	}
	allCommands = append(allCommands, localConfigCommands)

	userList := []string{}
	// creation of users / secrets can happen simultaneously
	userSecretCreateCommands := make(AsyncCommandSet, 0, *numberUsers+*numberSecrets)
	for i := 0; i < *numberUsers; i++ {
		name := fake.EmailAddress()
		userList = append(userList, name)
		pass, _ := uuid.NewV4()
		userSecretCreateCommands = append(userSecretCreateCommands, &UserCreateCommand{
			Name: name,
			Pass: pass.String(),
		})
	}

	secretTreeRoot := buildTreeRoot()
	for i := 0; i < *numberSecrets; i++ {
		secretName := fake.IPv4()
		secretNode := NewNode(secretName, nil)
		addNodeToTree(secretTreeRoot, secretNode)
		secretPath := getNodePath(secretNode, "/")

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
}

func initTenant() error {
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

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(asBytes))
	if err != nil {
		fmt.Println("failed to post to tenant create endpoint")
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		fmt.Println("Failed request")
		return errors.New("Failed to create tenant")
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read delete request response")
		return err
	} else {
		fmt.Println("response: ", string(respBody))
	}
	return nil
}
