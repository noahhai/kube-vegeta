package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/gobuffalo/uuid"
	"github.com/icrowley/fake"
)

var tenantName string
var initialAdmin LocalUser
var allCommands SyncCommandSet
var currDir string
var binaryName = "thy"

var (
	numberUsers       = flag.Int("users", 100, "Number of users to create")
	numberSecrets     = flag.Int("secrets", 500, "Number of secrets to create")
	numberPermissions = flag.Int("permissions", 50, "Unique Permissions Per User")
	secretLength      = flag.Int("secret-length", 100, "Length of each secret data")
)

func take(done <-chan interface{}, valueStream <-chan interface{}, num int) <-chan interface{} {
	takeStream := make(chan interface{})
	go func() {
		defer close(takeStream)
		for i := 0; i < num; i++ {
			select {
			case <-done:
				return
			case takeStream <- <-valueStream:
			}
		}
	}()
	return takeStream
}

func HandleCommands(cmdPipe <-chan Command, cancelPipe <-chan interface{}, resultPipe chan<- CmdResult) {
	for {
		select {
		case <-cancelPipe:
			return
		case c := <-cmdPipe:
			cmd := exec.Command(path.Join(currDir, binaryName), c.GetArgs()...)
			output, err := cmd.CombinedOutput()

			if err != nil {
				fmt.Println("Error executing command: " + strings.Join(c.GetArgs(), " "))
				fmt.Println(err)
			} else {
				// TODO : if we need output
				_ = output
				//resultPipe <- resultFromCmd(c.ResourceType, c.C)
			}
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
	cancelPipe := make(chan interface{})
	resultPipe := make(chan CmdResult)

	numWorkers := runtime.NumCPU()
	_ = numWorkers

	for i := 0; i < numWorkers; i++ {
		go HandleCommands(cmdPipe, cancelPipe, resultPipe)
	}

	for _, syncSetMember := range allCommands {
		for _, asyncCommand := range syncSetMember {
			cmdPipe <- asyncCommand
		}
	}

	return nil
}

func addConfigArg(args []string) []string {
	args = append(args, "--config")
	args = append(args, "config/.thy.yml")
	return args
}

func addLocalProfileArg(args []string) []string {
	args = append(args, "--profile")
	args = append(args, "local")
	return args
}

func main() {
	p, _ := os.Getwd()
	sep := ""
	for true {
		elems := strings.Split(p, string(filepath.Separator))
		f := elems[len(elems)-1]
		p = strings.Join(elems[:len(elems)-1], string(filepath.Separator))
		if f == "thy" {
			break
		} else if f == "" {
			os.Exit(1)
		}
		sep = sep + "../"
	}
	err := os.Chdir(sep)
	if err != nil {
		fmt.Printf("could not change dir: %v", err)
		os.Exit(1)
	}

	make := exec.Command("make")
	err = make.Run()
	if err != nil {
		fmt.Printf("could not make binary for %s: %v", binaryName, err)
		os.Exit(1)
	}
	status := 0
	err = DoSetup()
	if err != nil {
		fmt.Println(err)
		status = 1
	}
	os.Exit(status)
}

type Command interface {
	GetArgs() []string
	GetType() string
}

type UserCreateCommand struct {
	Name string
	Pass string
}

func (c *UserCreateCommand) GetType() string { return "user" }
func (c *UserCreateCommand) GetArgs() []string {
	return []string{
		"user",
		"create",
		"--username",
		c.Name,
		"password",
		c.Pass,
	}
}

type SecretCreateCommand struct {
	Path string
	Data string
}

func (c *SecretCreateCommand) GetType() string { return "secret" }
func (c *SecretCreateCommand) GetArgs() []string {
	return []string{
		"secret",
		"create",
		"--path",
		c.Path,
		"--data",
		c.Data,
	}
}

type PermissionCreateCommand struct {
	Path string
	User string
}

func (c *PermissionCreateCommand) GetType() string { return "permission" }
func (c *PermissionCreateCommand) GetArgs() []string {
	return []string{
		"secret",
		"permission",
		"create",
		"--subject",
		c.User,
		"--path",
		c.Path,
		"--action",
		"<read|delete|create|update>",
		"--effect",
		"allow",
	}
}

type CmdResult struct {
	Type   string
	Fields map[string]string
}

type AsyncCommandSet []Command

type SyncCommandSet []AsyncCommandSet

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

func init() {
	if dir, err := os.Getwd(); err != nil {
		panic(err)
	} else {
		currDir = dir
	}

	u, _ := uuid.NewV4()
	_ = u
	// TODO : Initial provisioning
	tenantName = "ambarco2"
	initialAdmin = LocalUser{
		Name: "noah",
		Pass: "noah@1",
	}

	allCommands = SyncCommandSet{}

	userCreateCommands := AsyncCommandSet{}
	allCommands = append(allCommands, userCreateCommands)

	secretCreateCommands := AsyncCommandSet{}
	allCommands = append(allCommands, secretCreateCommands)

	permissionCreateCommands := AsyncCommandSet{}
	allCommands = append(allCommands, permissionCreateCommands)

	userList := []string{}

	for i := 0; i < *numberUsers; i++ {
		name := fake.FullName()
		userList = append(userList, name)
		pass, _ := uuid.NewV4()
		userCreateCommands = append(userCreateCommands, &UserCreateCommand{
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

		secretCreateCommands = append(secretCreateCommands, &SecretCreateCommand{
			Path: secretPath,
			Data: data,
		})
	}

	rootFolders := getFirstLevelPaths(secretTreeRoot)
	for _, u := range userList {
		for _, p := range rootFolders {
			permissionCreateCommands = append(permissionCreateCommands, &PermissionCreateCommand{
				User: fmt.Sprintf("users:%s", u),
				Path: fmt.Sprintf("%s/*", p),
			})
		}
	}

}
