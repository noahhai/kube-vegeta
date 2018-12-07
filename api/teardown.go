package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func DoTeardown() error {
	// delete tenant
	client := &http.Client{}
	url := *adminEndpoint
	if !strings.HasSuffix(url, "/") {
		url = url + "/"
	}
	url = url + "tenant/" + *tenant
	fmt.Println("delete request to: " + url)

	req, err := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Basic NDhkZjg0NzEtYjRiNi00ZWU1LTlkN2MtNGMzYWYxM2ZhZThhOmQxMzFkY2M4LTRkYWEtNDg4MS1iNzgwLTU2ZTZmOWY3ZTE3YQ==")
	req.Header.Set("Content-Type", "application/json")

	if err != nil {
		fmt.Println("failed to create delete request")
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("failed to do delete request")
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read delete request response")
		return err
	} else {
		fmt.Println("response: " + string(respBody))

	}
	return nil
}
