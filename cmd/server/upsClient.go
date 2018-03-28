package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/common/log"
)

type upsClient struct {
	config *pushApplication
}

const BaseUrl = "http://localhost:8080/rest/applications"

// Find an Android Variant by it's Google Key
func (client *upsClient) hasAndroidVariant(key string) bool {
	url := fmt.Sprintf("%s/%s/android", BaseUrl, client.config.ApplicationId)
	log.Info("UPS request", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Error(err)

		// Return true here to prevent creating a new variant when the
		// request fails
		return true
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	variants := make([]androidVariant, 0)
	json.Unmarshal(body, &variants)

	for _, variant := range variants {
		if variant.GoogleKey == key {
			return true
		}
	}

	return false
}

func (client *upsClient) createAndroidVariant(variant *androidVariant) {
	url := fmt.Sprintf("%s/%s/android", BaseUrl, client.config.ApplicationId)
	log.Info("UPS request", url)

	payload, err := json.Marshal(variant)
	if err != nil {
		panic(err.Error())
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	httpClient := http.Client{}
	resp, err := httpClient.Do(req)

	if err != nil {
		panic(err.Error())
	}

	defer resp.Body.Close()
	log.Info("UPS response", resp)
}
