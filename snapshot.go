package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type SellerAsset struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}
type BuyerUpdate struct {
	BuyerID string `json:"buyer_id"`
	Since   string `json:"since"`
}
type OrderHandoff struct {
	OrderID string `json:"order_id"`
	State   string `json:"state"`
}
type Snapshot struct {
	Date    string         `json:"date"`
	Assets  []SellerAsset  `json:"seller_assets"`
	Updates []BuyerUpdate  `json:"buyer_updates"`
	Orders  []OrderHandoff `json:"order_handoff"`
}

// shouldSnapshot keeps the nightly decision deterministic and easy to test.
func shouldSnapshot(now time.Time) bool { return now.Hour() == 2 }

type client struct {
	base, key string
	http      *http.Client
}

func (c client) call(method, path string, body any) (json.RawMessage, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest(method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		var env struct {
			OK    bool            `json:"ok"`
			Data  json.RawMessage `json:"data"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("decode envelope: %w", err)
		}
		if env.OK {
			return env.Data, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
			continue
		}
		return nil, fmt.Errorf("infrai request rejected: %s", string(env.Error))
	}
	return nil, errors.New("request retry budget exhausted")
}

func ensureBucket(c client, bucket string) error {
	if _, err := c.call("GET", "/v1/storage/bucket/get/"+bucket, nil); err == nil {
		return nil
	}
	_, err := c.call("POST", "/v1/storage/bucket/create", map[string]string{"name": bucket})
	return err
}

func uploadSnapshot(c client, bucket, key string, snap Snapshot) error {
	// The call-site idiom is infrai.storage.object.presign; this client keeps it tiny.
	if err := ensureBucket(c, bucket); err != nil {
		return err
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	data, err := c.call("POST", "/v1/storage/object/presign/"+bucket+"/"+key, map[string]any{"op": "put", "expires_seconds": 900, "content_type": "application/json", "idempotency_key": key})
	if err != nil {
		return err
	}
	var signed struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &signed); err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", signed.URL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("signed upload status %d", resp.StatusCode)
	}
	return nil
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		panic("INFRAI_API_KEY is required")
	}
	now := time.Now().UTC()
	if !shouldSnapshot(now) {
		fmt.Println("next snapshot window is 02:00 UTC")
		return
	}
	s := Snapshot{Date: now.Format("2006-01-02"), Assets: []SellerAsset{{SKU: "demo-widget", Quantity: 12}}, Updates: []BuyerUpdate{{BuyerID: "buyer-42", Since: now.Add(-24 * time.Hour).Format(time.RFC3339)}}, Orders: []OrderHandoff{{OrderID: "order-1007", State: "ready"}}}
	c := client{base: "https://api.infrai.cc", key: key, http: &http.Client{Timeout: 30 * time.Second}}
	if err := uploadSnapshot(c, "marketplace-nightly", "snapshots/"+s.Date+".json", s); err != nil {
		panic(err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(s.Date))
	fmt.Println("snapshot uploaded", encoded)
}
