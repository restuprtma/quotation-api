package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const baseURL = "http://localhost:8080"
var token string
var companyID string
var clientID string

func main() {
	fmt.Println("=== E2E Test Runner ===")
	
	// 1. Auth
	if !login() { return }

	// 2. Create Quotation
	qID := createQuotation()
	if qID == "" { return }
	
	// 3. Test Revision
	newQID := createRevision(qID)
	if newQID == "" { return }

	// Legacy steps 3 (AI), 4 (Share/Artifacts), 7 (Public Sign) were removed in Phase 1.
	// We'll bring them back using the new domain architecture in Phase 11.

	fmt.Println("\n✅ E2E Phase 1 Test Completed Successfully!")
}

func doReq(method, path string, payload interface{}, includeAuth bool) ([]byte, error) {
	var bodyBytes []byte
	if payload != nil {
		bodyBytes, _ = json.Marshal(payload)
	}

	req, _ := http.NewRequest(method, baseURL+path, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if includeAuth {
		req.Header.Set("Authorization", "Bearer "+token)
		if companyID != "" {
			req.Header.Set("X-Company-ID", companyID)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func login() bool {
	fmt.Println("\n[1] Login owner@gmail.com...")
	body, err := doReq("POST", "/core/v1/auth/signin", map[string]string{
		"login":    "owner@gmail.com",
		"password": "Bismillah1407*",
	}, false)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	fmt.Println("Body:", string(body))
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	
	data := res["data"].(map[string]interface{})
	token = data["access_token"].(string)
	
	comp := data["company"].(map[string]interface{})
	companyID = comp["id"].(string)
	
	client := data["client"].(map[string]interface{})
	clientID = client["id"].(string)
	
	fmt.Println("✔ Logged in, got token and companyID:", companyID, "clientID:", clientID)
	return true
}

func createQuotation() string {
	fmt.Println("\n[2] Creating Product Quotation...")
	payload := map[string]interface{}{
		"client_id": clientID,
		"project_type": "product_based",
		"subject": "Test Phase 1 Subject",
		"budget": 100000,
	}
	body, err := doReq("POST", "/quotation/v1/quotations", payload, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	id := res["data"].(map[string]interface{})["id"].(string)
	fmt.Println("✔ Quotation Created, ID:", id)
	return id
}

func createRevision(id string) string {
	fmt.Println("\n[3] Creating Revision...")
	body, err := doReq("POST", "/quotation/v1/quotations/"+id+"/revisions", map[string]interface{}{}, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	newId := res["data"].(map[string]interface{})["id"].(string)
	fmt.Println("✔ Revision Created, New ID:", newId)
	return newId
}

