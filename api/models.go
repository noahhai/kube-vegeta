package main

type postLoaderModel struct {
	Tenant         string
	Domain         string
	Rate           int
	Duration       int
	SecretPaths    []string
	Tokens         []string
	StaticTargeter bool
	Workers        int
}
