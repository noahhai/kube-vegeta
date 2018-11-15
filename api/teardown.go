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
