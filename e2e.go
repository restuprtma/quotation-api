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

	// 3. Generate Mock AI
	if !generateMockAI(qID) { return }

	// 4. Share Quotation
	token := shareQuotation(qID)
	if token == "" { return }
	
	// 5. Test Revision
	newQID := createRevision(qID)
	if newQID == "" { return }

	// 6. Share Revision
	token2 := shareQuotation(newQID)
	if token2 == "" { return }

	// 7. Sign Quotation
	if !signQuotation(token2) { return }

	// 8. Create Contract
	cID := createContract(qID) // From signed original quotation
	if cID == "" { return }

	// 8. Share Contract
	cToken := shareContract(cID)
	if cToken == "" { return }

	// 9. Sign Contract
	if !signContract(cToken) { return }

	fmt.Println("\n✅ E2E Phase 11 Test Completed Successfully!")
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
		"budget": 100000,
		"sections": []map[string]interface{}{
			{
				"section_name": "Phase 1",
				"section_type": "package",
				"items": []map[string]interface{}{
					{"item_name": "Item 1", "pricing_mode": "unit", "billing_period": "one_time", "unit_price": 4000, "quantity": 1},
				},
			},
		},
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

func generateMockAI(id string) bool {
	fmt.Println("\n[3] Generating Mock AI Bundle...")
	_, err := doReq("POST", "/quotation/v1/ai/generate", map[string]interface{}{
		"project_type": "product_based",
		"client_name": "Test Client",
		"client_industry": "Tech",
		"subject": "Test subject",
		"client_requirements": []string{"fast", "reliable"},
	}, true)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	fmt.Println("✔ AI Mock Generated")
	return true
}

func shareQuotation(id string) string {
	fmt.Println("\n[4] Sharing Quotation...")
	_, err := doReq("POST", "/quotation/v1/quotations/"+id+"/share", map[string]interface{}{}, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	
	// Fetch artifacts
	body, err := doReq("GET", "/quotation/v1/quotations/"+id+"/artifacts", nil, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	
	data := res["data"].([]interface{})
	var qToken string
	for _, item := range data {
		art := item.(map[string]interface{})
		if art["artifact_type"].(string) == "quotation" {
			qToken = art["public_token"].(string)
		}
	}
	
	fmt.Println("✔ Shared, Public Token:", qToken)
	return qToken
}

func signQuotation(qToken string) bool {
	fmt.Println("\n[5] Signing Quotation (Public)...")
	_, err := doReq("POST", "/p/q/"+qToken+"/sign", map[string]interface{}{
		"signature_image": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAACklEQVR4nGMAAQAABQABDQottAAAAABJRU5ErkJggg==",
	}, false)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	fmt.Println("✔ Quotation Signed!")
	return true
}

func createRevision(id string) string {
	fmt.Println("\n[6] Creating Revision...")
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

func createContract(qId string) string {
	fmt.Println("\n[7] Creating Contract...")
	body, err := doReq("POST", "/contract/v1/contracts", map[string]interface{}{
		"quotation_id": qId,
	}, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	cId := res["data"].(map[string]interface{})["id"].(string)
	fmt.Println("✔ Contract Created, ID:", cId)
	return cId
}

func shareContract(cId string) string {
	fmt.Println("\n[8] Sharing Contract...")
	body, err := doReq("POST", "/contract/v1/contracts/"+cId+"/share", map[string]interface{}{}, true)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var res map[string]interface{}
	json.Unmarshal(body, &res)
	token := res["data"].(map[string]interface{})["public_token"].(string)
	fmt.Println("✔ Shared Contract, Token:", token)
	return token
}

func signContract(token string) bool {
	fmt.Println("\n[9] Signing Contract (Public)...")
	_, err := doReq("POST", "/p/c/"+token+"/sign", map[string]interface{}{
		"signature_data_url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAACklEQVR4nGMAAQAABQABDQottAAAAABJRU5ErkJggg==",
	}, false)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	fmt.Println("✔ Contract Signed!")
	return true
}
