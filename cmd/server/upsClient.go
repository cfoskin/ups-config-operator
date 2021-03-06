package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type upsClient struct {
	config *pushApplication
}

const BaseUrl = "http://localhost:8080/rest/applications"

// Delete the variant with the given google key
func (client *upsClient) deleteVariant(key string) bool {
	variant := client.hasAndroidVariant(key)
	if variant != nil {
		log.Printf("Deleting variant with id `%s`", variant.VariantID)

		url := fmt.Sprintf("%s/%s/adm/%s", BaseUrl, client.config.ApplicationId, variant.VariantID)
		log.Printf("UPS request", url)

		req, err := http.NewRequest(http.MethodDelete, url, nil)

		httpClient := http.Client{}
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Fatal(err.Error())
			return false
		}

		log.Printf("Variant `%s` has been deleted", variant.VariantID)
		return resp.StatusCode == 204
	}

	log.Printf("No variant found to delete (google key: `%s`)", key)
	return false
}

// Find an Android Variant by it's Google Key
func (client *upsClient) hasAndroidVariant(key string) *androidVariant {
	url := fmt.Sprintf("%s/%s/android", BaseUrl, client.config.ApplicationId)
	log.Printf("UPS request", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)

		// Return true here to prevent creating a new variant when the
		// request fails
		return &androidVariant{}
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	variants := make([]androidVariant, 0)
	json.Unmarshal(body, &variants)

	for _, variant := range variants {
		if variant.GoogleKey == key {
			return &variant
		}
	}

	return nil
}

func (client *upsClient) createAndroidVariant(variant *androidVariant) (bool, *androidVariant) {
	url := fmt.Sprintf("%s/%s/android", BaseUrl, client.config.ApplicationId)
	log.Printf("UPS request", url)

	payload, err := json.Marshal(variant)
	if err != nil {
		panic(err.Error())
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	httpClient := http.Client{}
	resp, err := httpClient.Do(req)

	if err != nil {
		panic(err.Error())
	}

	log.Printf("UPS responded with status code ", resp.Status)

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var createdVariant androidVariant
	json.Unmarshal(body, &createdVariant)

	return resp.StatusCode == 201, &createdVariant
}
