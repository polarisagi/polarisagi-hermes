package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// Let's send a fake request to Vertex AI to see if it complains about 400 before 401
	url := "https://us-central1-aiplatform.googleapis.com/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-pro:streamGenerateContent"
	body := []byte(`{
		"contents": [{"role": "user", "parts": [{"text": "Hello"}]}],
		"tools": [{
			"functionDeclarations": [{
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {
					"type": "OBJECT",
					"properties": {
						"loc": {"type": "STRING"}
					}
				}
			}]
		}]
	}`)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer ya29.fake")
	req.Header.Set("Content-Type", "application/json")
	
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	fmt.Println("Status:", resp.StatusCode)
	fmt.Println("Body:", string(b))
}
