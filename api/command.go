package main

import "encoding/json"

type Command interface {
	GetArgs() []string
	GetType() string
}

type BaseCommand struct {
	Args []string
}

func (c *BaseCommand) GetType() string { return "base" }
func (c *BaseCommand) GetArgs() []string {
	return c.Args
}

type ConfigCommand struct {
	Path string
	Val  string
}

func (c *ConfigCommand) GetType() string { return "config" }
func (c *ConfigCommand) GetArgs() []string {
	return []string{
		"config",
		"update",
		c.Path,
		c.Val,
	}
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
		"--password",
		c.Pass,
	}
}

type TokenCreateCommand struct {
	User string
}

func (c *TokenCreateCommand) GetType() string { return "token" }
func (c *TokenCreateCommand) GetArgs() []string {
	return []string{
		"auth",
		"-u",
		c.User,
		"-p",
		c.User + "@1",
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
	Type  string
	Value string
}

type TokenResult struct {
	AccessToken  string
	ExpiresIn    int
	Granted      string
	RefreshToken string
	TokenType    string
}

func GetTokenResult(output []byte) *CmdResult {
	var tr TokenResult
	if err := json.Unmarshal(output, &tr); err == nil {
		return &CmdResult{
			Type:  "token",
			Value: tr.AccessToken,
		}
	}
	return nil
}

type AsyncCommandSet []Command

type SyncCommandSet []AsyncCommandSet
