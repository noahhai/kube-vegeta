package main

type Command interface {
	GetArgs() []string
	GetType() string
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
